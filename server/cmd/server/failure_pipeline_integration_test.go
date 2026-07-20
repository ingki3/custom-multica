package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type failurePipelineFixture struct {
	issueID   string
	agentID   string
	runtimeID string
	taskID    string
}

func setupFailurePipelineFixture(t *testing.T, taskStatus, issueStatus, reason string, attempt, maxAttempts int) failurePipelineFixture {
	t.Helper()
	ctx := context.Background()
	var f failurePipelineFixture
	if err := testPool.QueryRow(ctx, `
		SELECT a.id, a.runtime_id FROM agent a
		JOIN member m ON m.workspace_id = a.workspace_id
		JOIN "user" u ON u.id = m.user_id
		WHERE u.email = $1 AND a.runtime_id IS NOT NULL
		LIMIT 1
	`, integrationTestEmail).Scan(&f.agentID, &f.runtimeID); err != nil {
		t.Fatalf("load pipeline test agent: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_type, creator_id, assignee_type, assignee_id)
		SELECT $1, 'Failure pipeline integration', $2, 'none', 'member', m.user_id, 'agent', $3
		FROM member m WHERE m.workspace_id = $1 LIMIT 1
		RETURNING id
	`, testWorkspaceID, issueStatus, f.agentID).Scan(&f.issueID); err != nil {
		t.Fatalf("create pipeline issue: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_task_queue (
			agent_id, runtime_id, issue_id, status, priority, dispatched_at,
			started_at, completed_at, error, failure_reason, attempt, max_attempts
		) VALUES (
			$1, $2, $3, $4, 0, now() - interval '1 minute',
			now() - interval '1 minute', CASE WHEN $4 = 'failed' THEN now() ELSE NULL END,
			'pipeline failure', $5, $6, $7
		) RETURNING id
	`, f.agentID, f.runtimeID, f.issueID, taskStatus, reason, attempt, maxAttempts).Scan(&f.taskID); err != nil {
		t.Fatalf("create pipeline task: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE issue_id = $1`, f.issueID)
		testPool.Exec(ctx, `DELETE FROM issue WHERE id = $1`, f.issueID)
		testPool.Exec(ctx, `UPDATE agent SET status = 'idle' WHERE id = $1`, f.agentID)
	})
	return f
}

func loadPipelineTask(t *testing.T, queries *db.Queries, taskID string) db.AgentTaskQueue {
	t.Helper()
	task, err := queries.GetAgentTask(context.Background(), parseUUID(taskID))
	if err != nil {
		t.Fatalf("load pipeline task: %v", err)
	}
	return task
}

func TestFailurePipelineRetryAndExhaustedFinal(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	tests := []struct {
		name         string
		attempt      int
		maxAttempts  int
		wantRetry    bool
		wantIssue    string
		wantComments int
	}{
		{name: "same-agent retry is intermediate", attempt: 1, maxAttempts: 3, wantRetry: true, wantIssue: "in_progress", wantComments: 0},
		{name: "exhausted retry is final", attempt: 3, maxAttempts: 3, wantIssue: "todo", wantComments: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			f := setupFailurePipelineFixture(t, "failed", "in_progress", "timeout", tt.attempt, tt.maxAttempts)
			queries := db.New(testPool)
			bus := events.New()
			svc := service.NewTaskService(queries, testPool, nil, bus)
			var internal, external []events.Event
			bus.Subscribe(protocol.EventTaskFailed, func(e events.Event) {
				if e.TaskID == "" || e.TaskID == f.taskID {
					internal = append(internal, e)
				}
			})
			bus.Subscribe(service.EventTaskFailed, func(e events.Event) {
				if e.TaskID == f.taskID {
					external = append(external, e)
				}
			})

			dispositions := svc.HandleFailedTasks(ctx, []db.AgentTaskQueue{loadPipelineTask(t, queries, f.taskID)})
			if len(dispositions) != 1 {
				t.Fatalf("got %d dispositions, want 1", len(dispositions))
			}
			d := dispositions[0]
			if d.WillRetry != tt.wantRetry || d.FinalFailure == tt.wantRetry || d.DecisionError != nil {
				t.Fatalf("unexpected disposition: %#v", d)
			}
			if tt.wantRetry && (d.RetryTask == nil || d.RetryKind != service.RetryKindSameAgent) {
				t.Fatalf("expected same-agent retry, got %#v", d)
			}

			var issueStatus string
			if err := testPool.QueryRow(ctx, `SELECT status FROM issue WHERE id = $1`, f.issueID).Scan(&issueStatus); err != nil {
				t.Fatal(err)
			}
			if issueStatus != tt.wantIssue {
				t.Fatalf("issue status = %q, want %q", issueStatus, tt.wantIssue)
			}
			var comments int
			if err := testPool.QueryRow(ctx, `SELECT count(*) FROM comment WHERE issue_id = $1`, f.issueID).Scan(&comments); err != nil {
				t.Fatal(err)
			}
			if comments != tt.wantComments {
				t.Fatalf("failure comments = %d, want %d", comments, tt.wantComments)
			}
			if len(internal) != 1 || len(external) != 1 {
				t.Fatalf("task failure event counts internal=%d external=%d, want exactly one each", len(internal), len(external))
			}
			payload := external[0].Payload.(map[string]any)
			if got := payload["issue"].(map[string]any)["status"]; got != tt.wantIssue {
				t.Fatalf("external payload issue status = %q, want %q", got, tt.wantIssue)
			}
			if got := payload["task"].(map[string]any)["will_retry"]; got != tt.wantRetry {
				t.Fatalf("external payload will_retry = %#v", got)
			}
		})
	}
}

func TestFailurePipelineFinalDoesNotRollbackWithActiveSibling(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	ctx := context.Background()
	f := setupFailurePipelineFixture(t, "failed", "in_progress", "agent_error", 1, 1)
	if _, err := testPool.Exec(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority, dispatched_at, started_at)
		VALUES ($1, $2, $3, 'running', 0, now(), now())
	`, f.agentID, f.runtimeID, f.issueID); err != nil {
		t.Fatalf("create active sibling: %v", err)
	}
	queries := db.New(testPool)
	svc := service.NewTaskService(queries, testPool, nil, events.New())
	d := svc.HandleFailedTasks(ctx, []db.AgentTaskQueue{loadPipelineTask(t, queries, f.taskID)})
	if len(d) != 1 || !d[0].FinalFailure {
		t.Fatalf("unexpected disposition: %#v", d)
	}
	var status string
	if err := testPool.QueryRow(ctx, `SELECT status FROM issue WHERE id = $1`, f.issueID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "in_progress" {
		t.Fatalf("active sibling issue status = %q, want in_progress", status)
	}
}

func TestFailTaskRunsFailurePipelineExactlyOnce(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	ctx := context.Background()
	f := setupFailurePipelineFixture(t, "running", "in_progress", "agent_error", 1, 1)
	queries := db.New(testPool)
	bus := events.New()
	svc := service.NewTaskService(queries, testPool, nil, bus)
	internal, external := 0, 0
	bus.Subscribe(protocol.EventTaskFailed, func(e events.Event) {
		if payload, ok := e.Payload.(map[string]any); ok && payload["task_id"] == f.taskID {
			internal++
		}
	})
	bus.Subscribe(service.EventTaskFailed, func(e events.Event) {
		if e.TaskID == f.taskID {
			external++
		}
	})
	for i := 0; i < 2; i++ {
		if _, err := svc.FailTask(ctx, parseUUID(f.taskID), "direct failure", "", "", "agent_error"); err != nil {
			t.Fatalf("FailTask call %d: %v", i+1, err)
		}
	}
	var comments int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM comment WHERE issue_id = $1`, f.issueID).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if internal != 1 || external != 1 || comments != 1 {
		t.Fatalf("direct pipeline counts internal=%d external=%d comments=%d, want 1/1/1", internal, external, comments)
	}
}

func TestHandleFailedTasksClaimsFailureOnceConcurrently(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	ctx := context.Background()
	f := setupFailurePipelineFixture(t, "failed", "in_progress", "agent_error", 1, 1)
	queries := db.New(testPool)
	failed := loadPipelineTask(t, queries, f.taskID)
	bus := events.New()
	svc := service.NewTaskService(queries, testPool, nil, bus)
	var internal, external atomic.Int32
	bus.Subscribe(protocol.EventTaskFailed, func(e events.Event) {
		if payload, ok := e.Payload.(map[string]any); ok && payload["task_id"] == f.taskID {
			internal.Add(1)
		}
	})
	bus.Subscribe(service.EventTaskFailed, func(e events.Event) {
		if e.TaskID == f.taskID {
			external.Add(1)
		}
	})

	start := make(chan struct{})
	results := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- len(svc.HandleFailedTasks(ctx, []db.AgentTaskQueue{failed}))
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	dispositions := 0
	for count := range results {
		dispositions += count
	}

	var comments, children int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM comment WHERE issue_id = $1`, f.issueID).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE parent_task_id = $1`, f.taskID).Scan(&children); err != nil {
		t.Fatal(err)
	}
	if dispositions != 1 || internal.Load() != 1 || external.Load() != 1 || comments != 1 || children != 0 {
		t.Fatalf("concurrent processing dispositions=%d internal=%d external=%d comments=%d children=%d", dispositions, internal.Load(), external.Load(), comments, children)
	}
}

func TestFailurePipelineRetryDecisionErrorIsNotFinal(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	ctx := context.Background()
	f := setupFailurePipelineFixture(t, "failed", "in_progress", "timeout", 1, 3)
	queries := db.New(testPool)
	failed := loadPipelineTask(t, queries, f.taskID)
	// Simulate a retry write race/failure after the sweeper has returned the
	// freshly failed row. CreateRetryTask now returns no rows.
	if _, err := testPool.Exec(ctx, `DELETE FROM agent_task_queue WHERE id = $1`, f.taskID); err != nil {
		t.Fatal(err)
	}
	bus := events.New()
	svc := service.NewTaskService(queries, testPool, nil, bus)
	internal, external := 0, 0
	bus.Subscribe(protocol.EventTaskFailed, func(e events.Event) {
		if payload, ok := e.Payload.(map[string]any); ok && payload["task_id"] == f.taskID {
			internal++
		}
	})
	bus.Subscribe(service.EventTaskFailed, func(e events.Event) {
		if e.TaskID == f.taskID {
			external++
		}
	})

	d := svc.HandleFailedTasks(ctx, []db.AgentTaskQueue{failed})
	if len(d) != 1 || d[0].DecisionError == nil || d[0].WillRetry || d[0].FinalFailure {
		t.Fatalf("retry write error was misclassified: %#v", d)
	}
	var status string
	if err := testPool.QueryRow(ctx, `SELECT status FROM issue WHERE id = $1`, f.issueID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	var comments int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM comment WHERE issue_id = $1`, f.issueID).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if status != "in_progress" || comments != 0 || internal != 1 || external != 0 {
		t.Fatalf("decision-error effects status=%q comments=%d internal=%d external=%d", status, comments, internal, external)
	}
}

func TestFailurePipelineFallbackIsIntermediate(t *testing.T) {
	if testPool == nil {
		t.Skip("no database connection")
	}
	ctx := context.Background()
	f := setupFailurePipelineFixture(t, "failed", "in_progress", "context_limit", 1, 3)
	var fallbackID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (workspace_id, name, runtime_mode, runtime_config, visibility, status, max_concurrent_tasks, owner_id, runtime_id)
		SELECT workspace_id, 'Failure fallback ' || gen_random_uuid(), runtime_mode, runtime_config, visibility, 'idle', max_concurrent_tasks, owner_id, runtime_id
		FROM agent WHERE id = $1 RETURNING id
	`, f.agentID).Scan(&fallbackID); err != nil {
		t.Fatalf("create fallback agent: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
		UPDATE agent SET fallback_agent_id = $2, fallback_failure_reasons = ARRAY['context_limit'], fallback_max_depth = 1
		WHERE id = $1
	`, f.agentID, fallbackID); err != nil {
		t.Fatalf("configure fallback: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(ctx, `UPDATE agent SET fallback_agent_id = NULL, fallback_failure_reasons = ARRAY['context_limit','rate_limit','model_limit'], fallback_max_depth = 1 WHERE id = $1`, f.agentID)
		testPool.Exec(ctx, `DELETE FROM agent WHERE id = $1`, fallbackID)
	})

	queries := db.New(testPool)
	svc := service.NewTaskService(queries, testPool, nil, events.New())
	d := svc.HandleFailedTasks(ctx, []db.AgentTaskQueue{loadPipelineTask(t, queries, f.taskID)})
	if len(d) != 1 || !d[0].WillRetry || d[0].RetryKind != service.RetryKindFallbackAgent || d[0].FinalFailure {
		t.Fatalf("unexpected fallback disposition: %#v", d)
	}
	if d[0].RetryTask == nil || d[0].RetryTask.AgentID != parseUUID(fallbackID) {
		t.Fatalf("fallback child has wrong agent: %#v", d[0].RetryTask)
	}
	var comments int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM comment WHERE issue_id = $1`, f.issueID).Scan(&comments); err != nil {
		t.Fatal(err)
	}
	if comments != 0 {
		t.Fatalf("fallback intermediate failure created %d comments, want 0", comments)
	}
}
