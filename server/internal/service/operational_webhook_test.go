package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type fakeOperationalQueries struct {
	workspaces []db.Workspace
	issues     map[string]db.Issue
	err        error
}

func (f *fakeOperationalQueries) GetWorkspace(_ context.Context, id pgtype.UUID) (db.Workspace, error) {
	if f.err != nil {
		return db.Workspace{}, f.err
	}
	for _, workspace := range f.workspaces {
		if workspace.ID == id {
			return workspace, nil
		}
	}
	return db.Workspace{}, errors.New("workspace not found")
}

func (f *fakeOperationalQueries) GetIssue(_ context.Context, id pgtype.UUID) (db.Issue, error) {
	issue, ok := f.issues[util.UUIDToString(id)]
	if !ok {
		return db.Issue{}, errors.New("issue not found")
	}
	return issue, nil
}

func (f *fakeOperationalQueries) ListAllWorkspaces(context.Context) ([]db.Workspace, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.workspaces, nil
}

func operationalUUID(t *testing.T) pgtype.UUID {
	t.Helper()
	return util.MustParseUUID(uuid.NewString())
}

func captureOperationalEvents(bus *events.Bus, eventType string) *[]events.Event {
	captured := make([]events.Event, 0)
	bus.Subscribe(eventType, func(event events.Event) { captured = append(captured, event) })
	return &captured
}

func assertOperationalEvent(t *testing.T, event events.Event, eventType string, workspace db.Workspace) map[string]any {
	t.Helper()
	if event.Type != eventType || event.WorkspaceID != util.UUIDToString(workspace.ID) || event.ActorType != "system" {
		t.Fatalf("unexpected event routing: %#v", event)
	}
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T", event.Payload)
	}
	if err := ValidateWebhookEnvelope(payload); err != nil {
		t.Fatalf("invalid operational envelope: %v; payload=%#v", err, payload)
	}
	if payload["event"] != eventType || payload["event_type"] != eventType {
		t.Fatalf("event vocabulary mismatch: %#v", payload)
	}
	workspacePayload := payload["workspace"].(map[string]any)
	if workspacePayload["id"] != util.UUIDToString(workspace.ID) || workspacePayload["slug"] != workspace.Slug || workspacePayload["name"] != workspace.Name {
		t.Fatalf("workspace payload = %#v", workspacePayload)
	}
	return payload
}

func TestPublishRuntimeOfflinePayloadCountsAndEnvelope(t *testing.T) {
	workspace := db.Workspace{ID: operationalUUID(t), Slug: "alpha", Name: "Alpha", IssuePrefix: "ALP"}
	runtime := db.AgentRuntime{ID: operationalUUID(t), WorkspaceID: workspace.ID, Provider: "claude"}
	issue := db.Issue{ID: operationalUUID(t), WorkspaceID: workspace.ID, Number: 42, Title: "Recover me", Status: "in_progress"}
	queries := &fakeOperationalQueries{workspaces: []db.Workspace{workspace}, issues: map[string]db.Issue{util.UUIDToString(issue.ID): issue}}
	failed := []db.AgentTaskQueue{{IssueID: issue.ID}, {IssueID: issue.ID}, {}}
	bus := events.New()
	captured := captureOperationalEvents(bus, EventRuntimeOffline)

	if !PublishRuntimeOffline(context.Background(), queries, bus, runtime, failed) {
		t.Fatal("PublishRuntimeOffline returned false")
	}
	if len(*captured) != 1 {
		t.Fatalf("published %d events, want one per runtime transition", len(*captured))
	}
	payload := assertOperationalEvent(t, (*captured)[0], EventRuntimeOffline, workspace)
	if payload["failed_task_count"] != 3 || payload["affected_issue_count"] != 1 {
		t.Fatalf("offline counts = failed:%v affected:%v", payload["failed_task_count"], payload["affected_issue_count"])
	}
	runtimePayload := payload["runtime"].(map[string]any)
	if runtimePayload["id"] != util.UUIDToString(runtime.ID) || runtimePayload["provider"] != runtime.Provider {
		t.Fatalf("runtime payload = %#v", runtimePayload)
	}
	issues := payload["affected_issues"].([]map[string]any)
	if len(issues) != 1 || issues[0]["identifier"] != "ALP-42" || issues[0]["title"] != issue.Title {
		t.Fatalf("affected issues = %#v", issues)
	}
}

func TestPublishRuntimeRecoveredPayloadCountsAndEnvelope(t *testing.T) {
	workspace := db.Workspace{ID: operationalUUID(t), Slug: "beta", Name: "Beta"}
	runtime := db.AgentRuntime{ID: operationalUUID(t), WorkspaceID: workspace.ID, Provider: "codex"}
	issue := db.Issue{ID: operationalUUID(t), WorkspaceID: workspace.ID, Number: 7, Title: "Orphan", Status: "todo"}
	queries := &fakeOperationalQueries{workspaces: []db.Workspace{workspace}, issues: map[string]db.Issue{util.UUIDToString(issue.ID): issue}}
	orphaned := []db.AgentTaskQueue{{IssueID: issue.ID}, {}}
	dispositions := []FailureDisposition{{WillRetry: true}, {FinalFailure: true}, {DecisionError: errors.New("undecided")}}
	bootID := uuid.NewString()
	bus := events.New()
	captured := captureOperationalEvents(bus, EventRuntimeRecovered)

	if !PublishRuntimeRecovered(context.Background(), queries, bus, runtime, bootID, orphaned, dispositions) {
		t.Fatal("PublishRuntimeRecovered returned false")
	}
	if len(*captured) != 1 {
		t.Fatalf("published %d events, want one per recovery transition", len(*captured))
	}
	payload := assertOperationalEvent(t, (*captured)[0], EventRuntimeRecovered, workspace)
	if payload["orphan_count"] != 2 || payload["retried_count"] != 1 || payload["final_failure_count"] != 1 || payload["affected_issue_count"] != 1 {
		t.Fatalf("recovery counts = %#v", payload)
	}
	runtimePayload := payload["runtime"].(map[string]any)
	if runtimePayload["boot_id"] != bootID || runtimePayload["id"] != util.UUIDToString(runtime.ID) {
		t.Fatalf("runtime payload = %#v", runtimePayload)
	}
}

func TestPublishServerReadyOncePerWorkspaceWithValidEnvelope(t *testing.T) {
	workspaces := []db.Workspace{
		{ID: operationalUUID(t), Slug: "one", Name: "One"},
		{ID: operationalUUID(t), Slug: "two", Name: "Two"},
	}
	queries := &fakeOperationalQueries{workspaces: workspaces}
	bus := events.New()
	captured := captureOperationalEvents(bus, EventServerReady)
	startedAt := time.Date(2026, 7, 19, 10, 11, 12, 0, time.FixedZone("offset", 3600))
	info := ServerReadyInfo{BootID: uuid.NewString(), Version: "v1.2.3", BindAddress: "127.0.0.1:8080", StartedAt: startedAt}

	if err := PublishServerReady(context.Background(), queries, bus, info); err != nil {
		t.Fatalf("PublishServerReady: %v", err)
	}
	if len(*captured) != len(workspaces) {
		t.Fatalf("published %d events, want exactly one for each of %d workspaces", len(*captured), len(workspaces))
	}
	seenEventIDs := make(map[string]bool)
	for i, event := range *captured {
		payload := assertOperationalEvent(t, event, EventServerReady, workspaces[i])
		serverPayload := payload["server"].(map[string]any)
		if serverPayload["boot_id"] != info.BootID || serverPayload["version"] != info.Version || serverPayload["bind_address"] != info.BindAddress || serverPayload["started_at"] != "2026-07-19T09:11:12Z" {
			t.Fatalf("server payload = %#v", serverPayload)
		}
		eventID := payload["event_id"].(string)
		if seenEventIDs[eventID] {
			t.Fatalf("event ID reused across workspaces: %s", eventID)
		}
		seenEventIDs[eventID] = true
	}
}

func TestOperationalPublishDoesNotEmitWhenWorkspaceLookupFails(t *testing.T) {
	queries := &fakeOperationalQueries{err: errors.New("database unavailable")}
	bus := events.New()
	captured := captureOperationalEvents(bus, EventRuntimeOffline)
	if PublishRuntimeOffline(context.Background(), queries, bus, db.AgentRuntime{WorkspaceID: operationalUUID(t)}, nil) {
		t.Fatal("publish should report failure")
	}
	if len(*captured) != 0 {
		t.Fatalf("published %d events after lookup failure", len(*captured))
	}
	if err := PublishServerReady(context.Background(), queries, bus, ServerReadyInfo{}); err == nil {
		t.Fatal("expected server-ready workspace listing error")
	}
}
