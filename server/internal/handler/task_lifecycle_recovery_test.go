package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/service"
)

func recoveryTestHandler(bus *events.Bus) *Handler {
	h := *testHandler
	h.Bus = bus
	h.TaskService = service.NewTaskService(h.Queries, h.TxStarter, h.Hub, bus, h.DaemonHub)
	h.TaskService.WebhookService = h.WebhookService
	return &h
}

func callRecoverOrphans(t *testing.T, h *Handler, bootID any) (int, map[string]any) {
	t.Helper()
	req := newDaemonTokenRequest(http.MethodPost, "/api/daemon/runtimes/"+testRuntimeID+"/recover-orphans", map[string]any{"boot_id": bootID}, testWorkspaceID, "handler-test-daemon")
	req = withURLParam(req, "runtimeId", testRuntimeID)
	w := httptest.NewRecorder()
	h.RecoverOrphanedTasks(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
		}
	}
	return w.Code, body
}

func TestRecoverOrphanedTasksSameBootExactlyOnceAndDifferentBoot(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `UPDATE agent_runtime SET last_recovered_boot_id = NULL WHERE id = $1`, testRuntimeID); err != nil {
		t.Fatalf("reset recovery boot: %v", err)
	}

	bus := events.New()
	h := recoveryTestHandler(bus)
	published := 0
	var publishedBootIDs []string
	bus.Subscribe(service.EventRuntimeRecovered, func(event events.Event) {
		published++
		payload := event.Payload.(map[string]any)
		publishedBootIDs = append(publishedBootIDs, payload["runtime"].(map[string]any)["boot_id"].(string))
	})

	firstBoot := uuid.NewString()
	for attempt := 0; attempt < 2; attempt++ {
		status, body := callRecoverOrphans(t, h, firstBoot)
		if status != http.StatusOK {
			t.Fatalf("same-boot request %d status = %d, body=%#v", attempt+1, status, body)
		}
	}
	if published != 1 {
		t.Fatalf("same boot published %d runtime.recovered events, want exactly one", published)
	}

	secondBoot := uuid.NewString()
	status, body := callRecoverOrphans(t, h, secondBoot)
	if status != http.StatusOK {
		t.Fatalf("different-boot request status = %d, body=%#v", status, body)
	}
	if published != 2 {
		t.Fatalf("different boot left event count at %d, want two transitions", published)
	}
	if publishedBootIDs[0] != firstBoot || publishedBootIDs[1] != secondBoot {
		t.Fatalf("published boot IDs = %#v", publishedBootIDs)
	}

	var storedBoot string
	if err := testPool.QueryRow(ctx, `SELECT last_recovered_boot_id::text FROM agent_runtime WHERE id = $1`, testRuntimeID).Scan(&storedBoot); err != nil {
		t.Fatalf("read stored recovery boot: %v", err)
	}
	if storedBoot != secondBoot {
		t.Fatalf("stored boot = %q, want %q", storedBoot, secondBoot)
	}
}

func TestRecoverOrphanedTasksDuplicateBootDoesNotDuplicateRetrySideEffects(t *testing.T) {
	ctx := context.Background()
	if _, err := testPool.Exec(ctx, `UPDATE agent_runtime SET last_recovered_boot_id = NULL WHERE id = $1`, testRuntimeID); err != nil {
		t.Fatalf("reset recovery boot: %v", err)
	}

	var agentID string
	if err := testPool.QueryRow(ctx, `SELECT id::text FROM agent WHERE runtime_id = $1 LIMIT 1`, testRuntimeID).Scan(&agentID); err != nil {
		t.Fatalf("find recovery test agent: %v", err)
	}
	var issueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, assignee_type, assignee_id)
		VALUES ($1, 'Recovery exactly-once retry', 'in_progress', 'none', 'member', $2, 'agent', $3)
		RETURNING id::text
	`, testWorkspaceID, testUserID, agentID).Scan(&issueID); err != nil {
		t.Fatalf("create recovery test issue: %v", err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID) })

	var taskID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority, attempt, max_attempts, dispatched_at)
		VALUES ($1, $2, $3, 'dispatched', 0, 1, 3, now())
		RETURNING id::text
	`, agentID, testRuntimeID, issueID).Scan(&taskID); err != nil {
		t.Fatalf("create orphaned task: %v", err)
	}

	bus := events.New()
	h := recoveryTestHandler(bus)
	published := 0
	bus.Subscribe(service.EventRuntimeRecovered, func(events.Event) { published++ })
	bootID := uuid.NewString()

	status, first := callRecoverOrphans(t, h, bootID)
	if status != http.StatusOK || first["orphaned"] != float64(1) || first["retried"] != float64(1) {
		t.Fatalf("first recovery = status %d body %#v, want one orphan and one retry", status, first)
	}
	status, duplicate := callRecoverOrphans(t, h, bootID)
	if status != http.StatusOK || duplicate["orphaned"] != float64(0) || duplicate["retried"] != float64(0) {
		t.Fatalf("duplicate recovery = status %d body %#v, want no repeated processing", status, duplicate)
	}
	if published != 1 {
		t.Fatalf("duplicate boot published %d recovery events", published)
	}

	var retryChildren int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE parent_task_id = $1`, taskID).Scan(&retryChildren); err != nil {
		t.Fatalf("count retry children: %v", err)
	}
	if retryChildren != 1 {
		t.Fatalf("retry children = %d, want exactly one after duplicate request", retryChildren)
	}
}

func TestRecoverOrphanedTasksRejectsInvalidBootIDBeforeSideEffects(t *testing.T) {
	ctx := context.Background()
	sentinelBoot := uuid.NewString()
	if _, err := testPool.Exec(ctx, `UPDATE agent_runtime SET last_recovered_boot_id = $1 WHERE id = $2`, sentinelBoot, testRuntimeID); err != nil {
		t.Fatalf("set recovery boot sentinel: %v", err)
	}

	bus := events.New()
	h := recoveryTestHandler(bus)
	published := 0
	bus.Subscribe(service.EventRuntimeRecovered, func(events.Event) { published++ })
	status, _ := callRecoverOrphans(t, h, "not-a-uuid")
	if status != http.StatusBadRequest {
		t.Fatalf("invalid boot ID status = %d, want 400", status)
	}
	if published != 0 {
		t.Fatalf("invalid boot ID published %d recovery events", published)
	}

	var storedBoot string
	if err := testPool.QueryRow(ctx, `SELECT last_recovered_boot_id::text FROM agent_runtime WHERE id = $1`, testRuntimeID).Scan(&storedBoot); err != nil {
		t.Fatalf("read stored recovery boot: %v", err)
	}
	if storedBoot != sentinelBoot {
		t.Fatalf("invalid request changed stored boot to %q", storedBoot)
	}
}
