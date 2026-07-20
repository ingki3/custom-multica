package main

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
)

func remediationFixture(t *testing.T, failureReason string) (issueID, taskID, agentID string) {
	t.Helper()
	issueID = createIssue(t, "Task remediation integration test")
	if err := testPool.QueryRow(context.Background(), `
		SELECT id::text FROM agent WHERE workspace_id = $1 AND archived_at IS NULL ORDER BY created_at LIMIT 1
	`, testWorkspaceID).Scan(&agentID); err != nil {
		t.Fatalf("load fixture agent: %v", err)
	}
	var runtimeID string
	if err := testPool.QueryRow(context.Background(), `
		UPDATE agent SET status = 'idle' WHERE id = $1 RETURNING runtime_id::text
	`, agentID).Scan(&runtimeID); err != nil {
		t.Fatalf("prepare fixture agent: %v", err)
	}
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, failure_reason, error, completed_at)
		VALUES ($1, $2, $3, 'failed', $4, 'fixture failure', now()) RETURNING id::text
	`, agentID, runtimeID, issueID, failureReason).Scan(&taskID); err != nil {
		t.Fatalf("create failed task: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
	})
	return issueID, taskID, agentID
}

func TestRemediateFailureAtomicIdempotency(t *testing.T) {
	issueID, taskID, _ := remediationFixture(t, "timeout")
	path := "/api/issues/" + issueID + "/remediate-failure"
	body := map[string]any{
		"failed_task_id": taskID, "action": "rerun",
		"remediation_key": "race-" + taskID, "reason": "retry timed-out task",
	}

	const callers = 2
	statuses := make(chan int, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp := authRequest(t, http.MethodPost, path, body)
			statuses <- resp.StatusCode
			resp.Body.Close()
		}()
	}
	wg.Wait()
	close(statuses)
	seen := map[int]int{}
	for status := range statuses {
		seen[status]++
	}
	if seen[http.StatusAccepted] != 1 || seen[http.StatusOK] != 1 {
		t.Fatalf("race statuses = %v, want one 202 and one 200", seen)
	}

	var remediationID string
	if err := testPool.QueryRow(context.Background(), `
		SELECT id::text FROM task_remediation WHERE failed_task_id = $1
	`, taskID).Scan(&remediationID); err != nil {
		t.Fatalf("load remediation: %v", err)
	}
	var childCount, commentCount int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*) FROM agent_task_queue WHERE parent_task_id = $1
	`, taskID).Scan(&childCount); err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("[hermes-remediation:%s]%%", remediationID)
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*) FROM comment WHERE issue_id = $1 AND content LIKE $2
	`, issueID, marker).Scan(&commentCount); err != nil {
		t.Fatal(err)
	}
	if childCount != 1 || commentCount != 1 {
		t.Fatalf("child/comment counts = %d/%d, want 1/1", childCount, commentCount)
	}
}

func TestRemediateFailureRejectsUnsafeAndActive(t *testing.T) {
	t.Run("unsafe reason", func(t *testing.T) {
		issueID, taskID, _ := remediationFixture(t, "agent_error")
		resp := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
			"failed_task_id": taskID, "action": "rerun",
			"remediation_key": "unsafe-" + taskID, "reason": "unsafe fixture",
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("active task", func(t *testing.T) {
		issueID, taskID, agentID := remediationFixture(t, "timeout")
		var runtimeID string
		if err := testPool.QueryRow(context.Background(), `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(context.Background(), `
			INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status) VALUES ($1, $2, $3, 'running')
		`, agentID, runtimeID, issueID); err != nil {
			t.Fatal(err)
		}
		resp := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
			"failed_task_id": taskID, "action": "rerun",
			"remediation_key": "active-" + taskID, "reason": "active fixture",
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409", resp.StatusCode)
		}
	})
}

func TestRemediateFailureBlockAndAutomatedBudget(t *testing.T) {
	issueID, taskID, agentID := remediationFixture(t, "agent_error")
	path := "/api/issues/" + issueID + "/remediate-failure"
	resp := authRequest(t, http.MethodPost, path, map[string]any{
		"failed_task_id": taskID, "action": "block",
		"remediation_key": "block-" + taskID, "reason": "operator intervention required",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("block status = %d, want 202", resp.StatusCode)
	}

	var status string
	if err := testPool.QueryRow(context.Background(), `SELECT status FROM issue WHERE id = $1`, issueID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "blocked" {
		t.Fatalf("issue status = %q, want blocked", status)
	}

	var runtimeID, secondTaskID string
	if err := testPool.QueryRow(context.Background(), `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, failure_reason, completed_at)
		VALUES ($1, $2, $3, 'failed', 'timeout', now()) RETURNING id::text
	`, agentID, runtimeID, issueID).Scan(&secondTaskID); err != nil {
		t.Fatal(err)
	}
	resp = authRequest(t, http.MethodPost, path, map[string]any{
		"failed_task_id": secondTaskID, "action": "block",
		"remediation_key": "second-" + secondTaskID, "reason": "second automated attempt",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second automated remediation status = %d, want 409", resp.StatusCode)
	}
}

func TestRemediateFailureReassignsIssueAndClonesTask(t *testing.T) {
	issueID, taskID, sourceAgentID := remediationFixture(t, "model_access")
	var targetAgentID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent (workspace_id, name, runtime_mode, runtime_config, visibility, status, max_concurrent_tasks, owner_id, runtime_id)
		SELECT workspace_id, 'Remediation target ' || gen_random_uuid(), runtime_mode, runtime_config,
		       visibility, 'idle', max_concurrent_tasks, owner_id, runtime_id
		FROM agent WHERE id = $1 RETURNING id::text
	`, sourceAgentID).Scan(&targetAgentID); err != nil {
		t.Fatalf("create target agent: %v", err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM agent WHERE id = $1`, targetAgentID) })

	resp := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
		"failed_task_id": taskID, "action": "reassign-rerun", "target_agent_id": targetAgentID,
		"remediation_key": "reassign-" + taskID, "reason": "use a compatible model agent",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}

	var assignedAgentID, childAgentID, parentTaskID string
	if err := testPool.QueryRow(context.Background(), `SELECT assignee_id::text FROM issue WHERE id = $1`, issueID).Scan(&assignedAgentID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(context.Background(), `
		SELECT agent_id::text, parent_task_id::text FROM agent_task_queue WHERE parent_task_id = $1
	`, taskID).Scan(&childAgentID, &parentTaskID); err != nil {
		t.Fatal(err)
	}
	if assignedAgentID != targetAgentID || childAgentID != targetAgentID || parentTaskID != taskID {
		t.Fatalf("reassignment issue=%s child=%s parent=%s", assignedAgentID, childAgentID, parentTaskID)
	}
}

func TestRemediateFailureRejectsTargetAndScopeConflicts(t *testing.T) {
	t.Run("same target agent", func(t *testing.T) {
		issueID, taskID, sourceAgentID := remediationFixture(t, "model_access")
		resp := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
			"failed_task_id": taskID, "action": "reassign-rerun", "target_agent_id": sourceAgentID,
			"remediation_key": "same-" + taskID, "reason": "same target is unsafe",
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422", resp.StatusCode)
		}
	})

	t.Run("non-failed task", func(t *testing.T) {
		issueID, taskID, _ := remediationFixture(t, "timeout")
		if _, err := testPool.Exec(context.Background(), `UPDATE agent_task_queue SET status = 'completed' WHERE id = $1`, taskID); err != nil {
			t.Fatal(err)
		}
		resp := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
			"failed_task_id": taskID, "action": "rerun",
			"remediation_key": "completed-" + taskID, "reason": "must reject completed task",
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("task from another issue", func(t *testing.T) {
		issueID, _, _ := remediationFixture(t, "timeout")
		_, otherTaskID, _ := remediationFixture(t, "timeout")
		resp := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
			"failed_task_id": otherTaskID, "action": "rerun",
			"remediation_key": "cross-scope-" + otherTaskID, "reason": "must reject cross-issue task",
		})
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("same key with different operation", func(t *testing.T) {
		issueID, taskID, _ := remediationFixture(t, "agent_error")
		key := "conflict-" + taskID
		first := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
			"failed_task_id": taskID, "action": "block", "remediation_key": key, "reason": "first operation",
		})
		first.Body.Close()
		if first.StatusCode != http.StatusAccepted {
			t.Fatalf("first status = %d, want 202", first.StatusCode)
		}
		second := authRequest(t, http.MethodPost, "/api/issues/"+issueID+"/remediate-failure", map[string]any{
			"failed_task_id": taskID, "action": "rerun", "remediation_key": key, "reason": "conflicting operation",
		})
		defer second.Body.Close()
		if second.StatusCode != http.StatusConflict {
			t.Fatalf("conflict status = %d, want 409", second.StatusCode)
		}
	})
}
