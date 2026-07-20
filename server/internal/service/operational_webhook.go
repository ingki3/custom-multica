package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ServerReadyInfo describes one API process after its public listener has
// successfully bound. BootID and StartedAt remain stable for that process.
type ServerReadyInfo struct {
	BootID      string
	Version     string
	BindAddress string
	StartedAt   time.Time
}

// operationalQueries is the read-only database surface needed to construct
// operational events. Keeping this narrow also makes fan-out and payload
// contracts testable without a PostgreSQL server.
type operationalQueries interface {
	GetWorkspace(context.Context, pgtype.UUID) (db.Workspace, error)
	GetIssue(context.Context, pgtype.UUID) (db.Issue, error)
	ListAllWorkspaces(context.Context) ([]db.Workspace, error)
}

func workspaceEnvelope(workspace db.Workspace) map[string]any {
	return map[string]any{
		"id":   util.UUIDToString(workspace.ID),
		"slug": workspace.Slug,
		"name": workspace.Name,
	}
}

func operationalEnvelope(eventType string, workspace db.Workspace, occurredAt time.Time) map[string]any {
	return map[string]any{
		"schema_version": 1,
		"event":          eventType,
		"event_type":     eventType,
		"event_id":       uuid.NewString(),
		"occurred_at":    occurredAt.UTC().Format(time.RFC3339),
		"workspace":      workspaceEnvelope(workspace),
	}
}

func publishOperational(bus *events.Bus, eventType string, workspace db.Workspace, payload map[string]any) {
	if bus == nil {
		return
	}
	bus.Publish(events.Event{
		Type:        eventType,
		WorkspaceID: util.UUIDToString(workspace.ID),
		ActorType:   "system",
		Payload:     payload,
	})
}

// affectedIssuePayloads resolves a de-duplicated, safe issue summary for the
// supplied tasks. It deliberately excludes task prompts, output, and sessions.
func affectedIssuePayloads(ctx context.Context, queries operationalQueries, workspace db.Workspace, tasks []db.AgentTaskQueue) []map[string]any {
	issues := make([]map[string]any, 0)
	seen := make(map[string]struct{})
	prefix := workspace.IssuePrefix
	if prefix == "" {
		prefix = strings.ToUpper(workspace.Slug)
	}
	for _, task := range tasks {
		if !task.IssueID.Valid {
			continue
		}
		id := util.UUIDToString(task.IssueID)
		if _, exists := seen[id]; exists {
			continue
		}
		issue, err := queries.GetIssue(ctx, task.IssueID)
		if err != nil || issue.WorkspaceID != workspace.ID {
			continue
		}
		seen[id] = struct{}{}
		issues = append(issues, map[string]any{
			"id":         id,
			"identifier": fmt.Sprintf("%s-%d", prefix, issue.Number),
			"number":     issue.Number,
			"title":      issue.Title,
			"status":     issue.Status,
		})
	}
	return issues
}

// PublishRuntimeOffline emits exactly one operational event for a runtime row
// returned by MarkStaleRuntimesOffline. The SQL transition guard ensures a
// later sweep cannot publish the same offline transition again.
func PublishRuntimeOffline(ctx context.Context, queries operationalQueries, bus *events.Bus, runtime db.AgentRuntime, failedTasks []db.AgentTaskQueue) bool {
	workspace, err := queries.GetWorkspace(ctx, runtime.WorkspaceID)
	if err != nil {
		return false
	}
	affectedIssues := affectedIssuePayloads(ctx, queries, workspace, failedTasks)
	payload := runtimeOfflinePayload(workspace, runtime, len(failedTasks), affectedIssues, time.Now())
	publishOperational(bus, EventRuntimeOffline, workspace, payload)
	return true
}

func runtimeOfflinePayload(workspace db.Workspace, runtime db.AgentRuntime, failedTaskCount int, affectedIssues []map[string]any, occurredAt time.Time) map[string]any {
	payload := operationalEnvelope(EventRuntimeOffline, workspace, occurredAt)
	payload["runtime"] = map[string]any{
		"id":       util.UUIDToString(runtime.ID),
		"provider": runtime.Provider,
	}
	payload["failed_task_count"] = failedTaskCount
	payload["affected_issue_count"] = len(affectedIssues)
	payload["affected_issues"] = affectedIssues
	return payload
}

// PublishRuntimeRecovered emits the summary after orphan handling and retry
// decisions have completed. RecordRuntimeRecoveryBoot is responsible for
// admitting only the first request for a runtime/daemon boot transition.
func PublishRuntimeRecovered(ctx context.Context, queries operationalQueries, bus *events.Bus, runtime db.AgentRuntime, bootID string, orphaned []db.AgentTaskQueue, dispositions []FailureDisposition) bool {
	workspace, err := queries.GetWorkspace(ctx, runtime.WorkspaceID)
	if err != nil {
		return false
	}
	retried, final := 0, 0
	for _, disposition := range dispositions {
		if disposition.WillRetry {
			retried++
		}
		if disposition.FinalFailure {
			final++
		}
	}
	affectedIssues := affectedIssuePayloads(ctx, queries, workspace, orphaned)
	payload := runtimeRecoveredPayload(workspace, runtime, bootID, len(orphaned), retried, final, affectedIssues, time.Now())
	publishOperational(bus, EventRuntimeRecovered, workspace, payload)
	return true
}

func runtimeRecoveredPayload(workspace db.Workspace, runtime db.AgentRuntime, bootID string, orphanCount, retriedCount, finalFailureCount int, affectedIssues []map[string]any, occurredAt time.Time) map[string]any {
	payload := operationalEnvelope(EventRuntimeRecovered, workspace, occurredAt)
	payload["runtime"] = map[string]any{
		"id":       util.UUIDToString(runtime.ID),
		"provider": runtime.Provider,
		"boot_id":  bootID,
	}
	payload["orphan_count"] = orphanCount
	payload["retried_count"] = retriedCount
	payload["final_failure_count"] = finalFailureCount
	payload["affected_issue_count"] = len(affectedIssues)
	payload["affected_issues"] = affectedIssues
	return payload
}

// PublishServerReady fans one event out per current workspace. It is called
// only after the public listener binds successfully.
func PublishServerReady(ctx context.Context, queries operationalQueries, bus *events.Bus, info ServerReadyInfo) error {
	workspaces, err := queries.ListAllWorkspaces(ctx)
	if err != nil {
		return err
	}
	for _, workspace := range workspaces {
		payload := serverReadyPayload(workspace, info, time.Now())
		publishOperational(bus, EventServerReady, workspace, payload)
	}
	return nil
}

func serverReadyPayload(workspace db.Workspace, info ServerReadyInfo, occurredAt time.Time) map[string]any {
	payload := operationalEnvelope(EventServerReady, workspace, occurredAt)
	payload["server"] = map[string]any{
		"boot_id":      info.BootID,
		"version":      info.Version,
		"bind_address": info.BindAddress,
		"started_at":   info.StartedAt.UTC().Format(time.RFC3339),
	}
	return payload
}
