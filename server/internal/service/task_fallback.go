package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"github.com/multica-ai/multica/server/pkg/redact"
)

type FallbackContext struct {
	FromTaskID  string `json:"from_task_id"`
	FromAgentID string `json:"from_agent_id"`
	Reason      string `json:"reason"`
}

type TaskContextWithFallback struct {
	Fallback *FallbackContext `json:"fallback,omitempty"`
}

func fallbackReasonAllowed(reason string, allowed []string) bool {
	if reason == "" {
		reason = "agent_error"
	}
	for _, r := range allowed {
		if r == reason {
			return true
		}
	}
	return false
}

func fallbackProviderAllowed(reason, sourceProvider, fallbackProvider string) bool {
	sameProvider := sourceProvider != "" && fallbackProvider != "" && sourceProvider == fallbackProvider
	if !sameProvider {
		return true
	}

	switch reason {
	case "auth_expired", "quota_exceeded":
		return false
	default:
		return true
	}
}

func taskFailureReason(task db.AgentTaskQueue) string {
	if task.FailureReason.Valid && task.FailureReason.String != "" {
		return task.FailureReason.String
	}
	return "agent_error"
}

func sameUUID(a, b pgtype.UUID) bool {
	return a.Valid && b.Valid && util.UUIDToString(a) == util.UUIDToString(b)
}

func (s *TaskService) fallbackDepth(ctx context.Context, task db.AgentTaskQueue) int {
	depth := 0
	seen := map[string]bool{}
	cur := task
	for cur.ParentTaskID.Valid {
		parentKey := util.UUIDToString(cur.ParentTaskID)
		if parentKey == "" || seen[parentKey] {
			break
		}
		seen[parentKey] = true

		parent, err := s.Queries.GetAgentTask(ctx, cur.ParentTaskID)
		if err != nil {
			break
		}
		if !sameUUID(parent.AgentID, cur.AgentID) {
			depth++
		}
		cur = parent
		if depth > 10 {
			break
		}
	}
	return depth
}

// MaybeFallbackFailedTask creates a queued child task for a configured fallback
// agent when the parent failed for a policy-allowed reason. This is distinct
// from MaybeRetryFailedTask: retries keep the same agent/runtime for flaky
// infrastructure failures, while fallback hands limit-shaped failures to a
// different agent.
func (s *TaskService) MaybeFallbackFailedTask(ctx context.Context, parent db.AgentTaskQueue) (*db.AgentTaskQueue, error) {
	if parent.Status != "failed" {
		return nil, nil
	}
	if parent.AutopilotRunID.Valid {
		return nil, nil
	}
	if !parent.IssueID.Valid && !parent.ChatSessionID.Valid {
		return nil, nil
	}

	reason := taskFailureReason(parent)
	sourceAgent, err := s.Queries.GetAgent(ctx, parent.AgentID)
	if err != nil {
		return nil, fmt.Errorf("load source agent for fallback: %w", err)
	}
	if !sourceAgent.FallbackAgentID.Valid {
		return nil, nil
	}
	if sourceAgent.FallbackMaxDepth <= 0 || s.fallbackDepth(ctx, parent) >= int(sourceAgent.FallbackMaxDepth) {
		slog.Info("task fallback skipped: depth exhausted",
			"task_id", util.UUIDToString(parent.ID),
			"reason", reason,
			"fallback_max_depth", sourceAgent.FallbackMaxDepth,
		)
		return nil, nil
	}
	if !fallbackReasonAllowed(reason, sourceAgent.FallbackFailureReasons) {
		return nil, nil
	}

	fallbackAgent, err := s.Queries.GetAgent(ctx, sourceAgent.FallbackAgentID)
	if err != nil {
		return nil, fmt.Errorf("load fallback agent: %w", err)
	}
	if !sameUUID(sourceAgent.WorkspaceID, fallbackAgent.WorkspaceID) {
		slog.Warn("task fallback skipped: fallback agent belongs to another workspace",
			"task_id", util.UUIDToString(parent.ID),
			"source_agent_id", util.UUIDToString(sourceAgent.ID),
			"fallback_agent_id", util.UUIDToString(fallbackAgent.ID),
		)
		return nil, nil
	}
	if fallbackAgent.ArchivedAt.Valid || !fallbackAgent.RuntimeID.Valid {
		return nil, nil
	}
	if !sourceAgent.RuntimeID.Valid {
		return nil, nil
	}

	sourceRuntime, err := s.Queries.GetAgentRuntime(ctx, sourceAgent.RuntimeID)
	if err != nil {
		return nil, fmt.Errorf("load source runtime for fallback: %w", err)
	}
	fallbackRuntime, err := s.Queries.GetAgentRuntime(ctx, fallbackAgent.RuntimeID)
	if err != nil {
		return nil, fmt.Errorf("load fallback runtime: %w", err)
	}
	if !fallbackProviderAllowed(reason, sourceRuntime.Provider, fallbackRuntime.Provider) {
		slog.Info("task fallback skipped: same provider blocked for failure reason",
			"task_id", util.UUIDToString(parent.ID),
			"source_provider", sourceRuntime.Provider,
			"fallback_provider", fallbackRuntime.Provider,
			"reason", reason,
		)
		return nil, nil
	}

	child, err := s.Queries.CreateFallbackTask(ctx, db.CreateFallbackTaskParams{
		FallbackAgentID:   fallbackAgent.ID,
		FallbackRuntimeID: fallbackAgent.RuntimeID,
		ParentTaskID:      parent.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("create fallback task: %w", err)
	}

	slog.Info("task fallback enqueued",
		"parent_task_id", util.UUIDToString(parent.ID),
		"child_task_id", util.UUIDToString(child.ID),
		"source_agent_id", util.UUIDToString(sourceAgent.ID),
		"fallback_agent_id", util.UUIDToString(fallbackAgent.ID),
		"reason", reason,
	)

	if parent.IssueID.Valid {
		msg := fmt.Sprintf("Fallback triggered: original agent failed with `%s`; continuing with fallback agent `%s`.", reason, fallbackAgent.Name)
		s.createAgentComment(ctx, parent.IssueID, sourceAgent.ID, redact.Text(msg), "system", parent.TriggerCommentID)
	}

	s.broadcastTaskEvent(ctx, protocol.EventTaskQueued, child)
	s.notifyTaskAvailable(child)
	return &child, nil
}
