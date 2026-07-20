package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestNewFailureDisposition(t *testing.T) {
	parent := db.AgentTaskQueue{Status: "failed"}
	child := db.AgentTaskQueue{Status: "queued"}
	tests := []struct {
		name      string
		child     *db.AgentTaskQueue
		kind      RetryKind
		wantRetry bool
		wantFinal bool
	}{
		{name: "same-agent retry", child: &child, kind: RetryKindSameAgent, wantRetry: true},
		{name: "fallback-agent retry", child: &child, kind: RetryKindFallbackAgent, wantRetry: true},
		{name: "final exhausted", kind: RetryKindSameAgent, wantFinal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newFailureDisposition(parent, tt.child, tt.kind)
			if got.WillRetry != tt.wantRetry || got.FinalFailure != tt.wantFinal {
				t.Fatalf("got will_retry=%v final=%v", got.WillRetry, got.FinalFailure)
			}
			if tt.child == nil && got.RetryKind != RetryKindNone {
				t.Fatalf("final failure retry kind = %q, want none", got.RetryKind)
			}
			if tt.child != nil && got.RetryKind != tt.kind {
				t.Fatalf("retry kind = %q, want %q", got.RetryKind, tt.kind)
			}
		})
	}
}

func TestBoundedTaskFailureErrorRedactsAndTruncatesRunes(t *testing.T) {
	secret := "sk-abcdefghijklmnopqrstuvwxyz1234567890"
	raw := "failure " + secret + " " + strings.Repeat("界", 600)

	got := boundedTaskFailureError(raw)
	if strings.Contains(got, secret) {
		t.Fatal("error summary leaked API key")
	}
	if !strings.Contains(got, "[REDACTED API KEY]") {
		t.Fatalf("expected redaction marker, got %q", got)
	}
	if count := utf8.RuneCountInString(got); count != taskFailureErrorMaxRunes {
		t.Fatalf("expected exactly %d runes after truncation, got %d", taskFailureErrorMaxRunes, count)
	}
}

func TestBoundedTaskFailureErrorDoesNotPadShortText(t *testing.T) {
	const raw = "ordinary agent failure"
	if got := boundedTaskFailureError(raw); got != raw {
		t.Fatalf("got %q, want %q", got, raw)
	}
}

func TestBuildTaskFailedPayloadRetryAndCurrentIssueState(t *testing.T) {
	completed := time.Date(2026, 7, 19, 12, 34, 56, 0, time.UTC)
	secret := "sk-abcdefghijklmnopqrstuvwxyz123456"
	parent := db.AgentTaskQueue{
		ID: testUUID(11), AgentID: testUUID(12), RuntimeID: testUUID(13),
		IssueID: testUUID(14), Status: "failed", Attempt: 1, MaxAttempts: 3,
		CompletedAt:   pgtype.Timestamptz{Time: completed, Valid: true},
		FailureReason: pgtype.Text{String: "timeout", Valid: true},
		Error:         pgtype.Text{String: "token " + secret + " " + strings.Repeat("界", 600), Valid: true},
		Context:       []byte(`{"prompt":"TOP SECRET PROMPT"}`),
		SessionID:     pgtype.Text{String: "secret-session", Valid: true},
		WorkDir:       pgtype.Text{String: "/secret/workdir", Valid: true},
	}
	child := db.AgentTaskQueue{ID: testUUID(15), Status: "queued"}
	workspace := db.Workspace{ID: testUUID(16), Slug: "quality", Name: "Quality", IssuePrefix: "MUL"}
	issue := db.Issue{ID: parent.IssueID, Number: 42, Title: "Failure pipeline", Status: "todo", ProjectID: testUUID(17)}
	project := db.Project{ID: issue.ProjectID, Title: "Reliability"}

	payload := buildTaskFailedPayload(newFailureDisposition(parent, &child, RetryKindSameAgent), workspace, &issue, &project)
	taskPayload := payload["task"].(map[string]any)
	if taskPayload["will_retry"] != true || taskPayload["retry_kind"] != string(RetryKindSameAgent) {
		t.Fatalf("unexpected retry disposition: %#v", taskPayload)
	}
	if taskPayload["retry_task_id"] != uuidString(child.ID) {
		t.Fatalf("retry_task_id = %#v", taskPayload["retry_task_id"])
	}
	if got := payload["issue"].(map[string]any)["status"]; got != "todo" {
		t.Fatalf("issue status = %q, want current persisted status todo", got)
	}
	if _, ok := payload["project"]; !ok {
		t.Fatal("expected optional project when loaded")
	}
	if got := payload["occurred_at"]; got != completed.Format(time.RFC3339) {
		t.Fatalf("occurred_at = %q", got)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	public := string(encoded)
	for _, forbidden := range []string{secret, "TOP SECRET PROMPT", "secret-session", "/secret/workdir"} {
		if strings.Contains(public, forbidden) {
			t.Fatalf("public payload leaked %q: %s", forbidden, public)
		}
	}
	if !strings.Contains(public, "[REDACTED API KEY]") {
		t.Fatalf("public payload missed redaction marker: %s", public)
	}
	if got := utf8.RuneCountInString(taskPayload["error"].(string)); got != taskFailureErrorMaxRunes {
		t.Fatalf("payload error has %d runes, want %d", got, taskFailureErrorMaxRunes)
	}
}

func TestBuildTaskFailedPayloadFinalWithoutOptionalIssue(t *testing.T) {
	task := db.AgentTaskQueue{ID: testUUID(21), Status: "failed", Attempt: 3, MaxAttempts: 3}
	workspace := db.Workspace{ID: testUUID(22), Slug: "quality", Name: "Quality"}
	payload := buildTaskFailedPayload(newFailureDisposition(task, nil, RetryKindSameAgent), workspace, nil, nil)

	taskPayload := payload["task"].(map[string]any)
	if taskPayload["will_retry"] != false || taskPayload["retry_kind"] != string(RetryKindNone) {
		t.Fatalf("unexpected final disposition: %#v", taskPayload)
	}
	if taskPayload["retry_task_id"] != nil {
		t.Fatalf("final retry_task_id = %#v, want nil", taskPayload["retry_task_id"])
	}
	if _, ok := payload["issue"]; ok {
		t.Fatal("issue must be omitted when it cannot be loaded")
	}
	if _, ok := payload["project"]; ok {
		t.Fatal("project must be omitted without an issue")
	}
	ws := payload["workspace"].(map[string]any)
	if ws["id"] == "" {
		t.Fatal("workspace id must be valid and non-empty")
	}
}

func TestBuildTaskFailureRealtimePayloadIncludesDisposition(t *testing.T) {
	parent := db.AgentTaskQueue{ID: testUUID(31), AgentID: testUUID(32), IssueID: testUUID(33), Status: "failed"}
	child := db.AgentTaskQueue{ID: testUUID(34), Status: "queued"}

	retrying := buildTaskFailureRealtimePayload(newFailureDisposition(parent, &child, RetryKindFallbackAgent))
	if retrying["will_retry"] != true || retrying["final_failure"] != false {
		t.Fatalf("retrying payload lacks disposition: %#v", retrying)
	}
	if retrying["retry_kind"] != string(RetryKindFallbackAgent) || retrying["retry_task_id"] != uuidString(child.ID) {
		t.Fatalf("retrying payload has wrong retry metadata: %#v", retrying)
	}

	final := buildTaskFailureRealtimePayload(newFailureDisposition(parent, nil, RetryKindNone))
	if final["will_retry"] != false || final["final_failure"] != true {
		t.Fatalf("final payload lacks disposition: %#v", final)
	}
}

func uuidString(id pgtype.UUID) string {
	b, _ := id.Value()
	return b.(string)
}
