package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const (
	RemediationActionRerun         = "rerun"
	RemediationActionReassignRerun = "reassign_rerun"
	RemediationActionBlock         = "block"
)

// RemediationError is safe to expose at the HTTP boundary.
type RemediationError struct {
	Code    string
	Message string
}

func (e *RemediationError) Error() string { return e.Message }

func remediationError(code, message string) error {
	return &RemediationError{Code: code, Message: message}
}

type RemediateTaskFailureParams struct {
	WorkspaceID    pgtype.UUID
	IssueID        pgtype.UUID
	FailedTaskID   pgtype.UUID
	Action         string
	TargetAgentID  pgtype.UUID
	RemediationKey string
	Reason         string
	CreatedBy      pgtype.UUID
	Automated      bool
}

type RemediateTaskFailureResult struct {
	Remediation db.TaskRemediation
	CreatedTask *db.AgentTaskQueue
	Issue       db.Issue
	Comment     *db.Comment
	Idempotent  bool
}

var safeRerunReasons = map[string]bool{
	"timeout": true, "runtime_offline": true, "runtime_recovery": true,
}

var safeReassignReasons = map[string]bool{
	"rate_limit": true, "context_limit": true, "model_limit": true,
	"model_access": true, "model_not_found": true,
}

func remediationActionSafe(action, failureReason string) bool {
	switch action {
	case RemediationActionRerun:
		return safeRerunReasons[failureReason]
	case RemediationActionReassignRerun:
		return safeReassignReasons[failureReason]
	case RemediationActionBlock:
		return true
	default:
		return false
	}
}

func validateRemediationInput(p RemediateTaskFailureParams) error {
	p.RemediationKey = strings.TrimSpace(p.RemediationKey)
	p.Reason = strings.TrimSpace(p.Reason)
	if p.RemediationKey == "" || utf8.RuneCountInString(p.RemediationKey) > 200 {
		return remediationError("invalid_remediation_key", "remediation_key must be between 1 and 200 characters")
	}
	if p.Reason == "" || utf8.RuneCountInString(p.Reason) > 500 {
		return remediationError("invalid_reason", "reason must be between 1 and 500 characters")
	}
	for _, r := range p.Reason {
		if unicode.IsControl(r) {
			return remediationError("unsafe_reason", "reason must be a single-line operator-safe summary")
		}
	}
	if p.Action != RemediationActionRerun && p.Action != RemediationActionReassignRerun && p.Action != RemediationActionBlock {
		return remediationError("invalid_action", "action must be rerun, reassign-rerun, or block")
	}
	if p.Action == RemediationActionReassignRerun && !p.TargetAgentID.Valid {
		return remediationError("target_agent_required", "target_agent_id is required for reassign-rerun")
	}
	if p.Action != RemediationActionReassignRerun && p.TargetAgentID.Valid {
		return remediationError("unexpected_target_agent", "target_agent_id is only valid for reassign-rerun")
	}
	return nil
}

// RemediateTaskFailure atomically claims a final failure and applies exactly one
// remediation. The issue row lock serializes budget checks for all failed tasks
// belonging to the issue; database unique constraints provide a second guard.
func (s *TaskService) RemediateTaskFailure(ctx context.Context, p RemediateTaskFailureParams) (RemediateTaskFailureResult, error) {
	if err := validateRemediationInput(p); err != nil {
		return RemediateTaskFailureResult{}, err
	}
	p.RemediationKey = strings.TrimSpace(p.RemediationKey)
	p.Reason = strings.TrimSpace(p.Reason)
	if s.TxStarter == nil {
		return RemediateTaskFailureResult{}, fmt.Errorf("task remediation requires a transaction starter")
	}

	var result RemediateTaskFailureResult
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin remediation transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	issue, err := q.LockIssueForRemediation(ctx, db.LockIssueForRemediationParams{IssueID: p.IssueID, WorkspaceID: p.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, remediationError("issue_not_found", "issue not found")
	}
	if err != nil {
		return result, fmt.Errorf("lock issue: %w", err)
	}
	result.Issue = issue
	originalIssueStatus := issue.Status

	failed, err := q.GetFailedTaskForRemediation(ctx, db.GetFailedTaskForRemediationParams{
		FailedTaskID: p.FailedTaskID, IssueID: p.IssueID, WorkspaceID: p.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return result, remediationError("failed_task_not_found", "failed task does not belong to this issue and workspace")
	}
	if err != nil {
		return result, fmt.Errorf("lock failed task: %w", err)
	}
	if failed.Status != "failed" {
		return result, remediationError("task_not_failed", "task status must be failed")
	}

	// Resolve a replay after validating task scope/status, but before either
	// remediation budget check. Thus a replay remains successful even though
	// its first call created an active child and spent the automated budget.
	existing, err := q.GetTaskRemediationByKey(ctx, db.GetTaskRemediationByKeyParams{
		WorkspaceID: p.WorkspaceID, RemediationKey: p.RemediationKey,
	})
	if err == nil {
		if !sameUUID(existing.IssueID, p.IssueID) || !sameUUID(existing.FailedTaskID, p.FailedTaskID) || existing.Action != p.Action {
			return result, remediationError("remediation_key_conflict", "remediation_key is already used by a different remediation")
		}
		result.Remediation = existing
		result.Idempotent = true
		if existing.CreatedTaskID.Valid {
			if task, taskErr := q.GetAgentTask(ctx, existing.CreatedTaskID); taskErr == nil {
				result.CreatedTask = &task
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return RemediateTaskFailureResult{}, fmt.Errorf("commit idempotent remediation: %w", err)
		}
		return result, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, fmt.Errorf("check remediation key: %w", err)
	}
	if prior, priorErr := q.GetTaskRemediationByFailedTask(ctx, p.FailedTaskID); priorErr == nil {
		_ = prior
		return result, remediationError("failed_task_already_remediated", "failed task has already been remediated")
	} else if !errors.Is(priorErr, pgx.ErrNoRows) {
		return result, fmt.Errorf("check failed task remediation: %w", priorErr)
	}

	active, err := q.HasActiveTaskForRemediationIssue(ctx, p.IssueID)
	if err != nil {
		return result, fmt.Errorf("check active tasks: %w", err)
	}
	if active {
		return result, remediationError("active_task_exists", "issue already has an active task")
	}
	if p.Automated {
		used, usedErr := q.HasAutomatedTaskRemediation(ctx, p.IssueID)
		if usedErr != nil {
			return result, fmt.Errorf("check remediation budget: %w", usedErr)
		}
		if used {
			return result, remediationError("remediation_budget_exhausted", "issue already has an automated remediation")
		}
	}

	failureReason := taskFailureReason(failed)
	if !remediationActionSafe(p.Action, failureReason) {
		return result, remediationError("unsafe_remediation", fmt.Sprintf("action %s is not safe for failure reason %s", strings.ReplaceAll(p.Action, "_", "-"), failureReason))
	}

	targetAgentID := failed.AgentID
	var targetRuntimeID pgtype.UUID
	if p.Action != RemediationActionBlock {
		if p.Action == RemediationActionReassignRerun {
			targetAgentID = p.TargetAgentID
			if sameUUID(targetAgentID, failed.AgentID) {
				return result, remediationError("same_target_agent", "target agent must differ from the failed task agent")
			}
		}
		target, targetErr := q.GetAgent(ctx, targetAgentID)
		if errors.Is(targetErr, pgx.ErrNoRows) || (targetErr == nil && !sameUUID(target.WorkspaceID, p.WorkspaceID)) {
			return result, remediationError("target_agent_not_found", "target agent not found in workspace")
		}
		if targetErr != nil {
			return result, fmt.Errorf("load target agent: %w", targetErr)
		}
		if target.ArchivedAt.Valid {
			return result, remediationError("target_agent_archived", "target agent is archived")
		}
		if target.Status == "offline" {
			return result, remediationError("target_agent_offline", "target agent is offline")
		}
		if !target.RuntimeID.Valid {
			return result, remediationError("target_runtime_missing", "target agent has no runtime")
		}
		runtime, runtimeErr := q.GetAgentRuntimeForWorkspace(ctx, db.GetAgentRuntimeForWorkspaceParams{ID: target.RuntimeID, WorkspaceID: p.WorkspaceID})
		if errors.Is(runtimeErr, pgx.ErrNoRows) {
			return result, remediationError("target_runtime_missing", "target runtime not found in workspace")
		}
		if runtimeErr != nil {
			return result, fmt.Errorf("load target runtime: %w", runtimeErr)
		}
		if runtime.Status != "online" {
			return result, remediationError("target_runtime_offline", "target runtime is offline")
		}
		targetRuntimeID = runtime.ID
	}

	claim, err := q.CreateTaskRemediationClaim(ctx, db.CreateTaskRemediationClaimParams{
		WorkspaceID: p.WorkspaceID, IssueID: p.IssueID, FailedTaskID: p.FailedTaskID,
		SourceAgentID: failed.AgentID, TargetAgentID: p.TargetAgentID,
		RemediationKey: p.RemediationKey, Action: p.Action, Automated: p.Automated,
		Reason: p.Reason, CreatedBy: p.CreatedBy,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, existingErr := q.GetTaskRemediationByKey(ctx, db.GetTaskRemediationByKeyParams{
			WorkspaceID: p.WorkspaceID, RemediationKey: p.RemediationKey,
		})
		if existingErr != nil {
			return result, fmt.Errorf("reload concurrently claimed remediation: %w", existingErr)
		}
		if !sameUUID(existing.IssueID, p.IssueID) || !sameUUID(existing.FailedTaskID, p.FailedTaskID) || existing.Action != p.Action {
			return result, remediationError("remediation_key_conflict", "remediation_key is already used by a different remediation")
		}
		result.Remediation, result.Idempotent = existing, true
		if existing.CreatedTaskID.Valid {
			if task, taskErr := q.GetAgentTask(ctx, existing.CreatedTaskID); taskErr == nil {
				result.CreatedTask = &task
			}
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return RemediateTaskFailureResult{}, fmt.Errorf("commit concurrent idempotent remediation: %w", commitErr)
		}
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("claim remediation: %w", err)
	}

	if p.Action == RemediationActionBlock {
		issue, err = q.BlockIssueForRemediation(ctx, p.IssueID)
		if err != nil {
			return result, fmt.Errorf("block issue: %w", err)
		}
		result.Issue = issue
	} else {
		if p.Action == RemediationActionReassignRerun {
			issue, err = q.ReassignIssueForRemediation(ctx, db.ReassignIssueForRemediationParams{TargetAgentID: targetAgentID, IssueID: p.IssueID})
			if err != nil {
				return result, fmt.Errorf("reassign issue: %w", err)
			}
			result.Issue = issue
		}
		child, childErr := q.CreateTaskRemediationChild(ctx, db.CreateTaskRemediationChildParams{
			TargetAgentID: targetAgentID, TargetRuntimeID: targetRuntimeID, FailedTaskID: p.FailedTaskID,
		})
		if childErr != nil {
			return result, fmt.Errorf("clone remediation task: %w", childErr)
		}
		claim, err = q.SetTaskRemediationCreatedTask(ctx, db.SetTaskRemediationCreatedTaskParams{CreatedTaskID: child.ID, ID: claim.ID})
		if err != nil {
			return result, fmt.Errorf("link remediation task: %w", err)
		}
		result.CreatedTask = &child
	}

	marker := fmt.Sprintf("[hermes-remediation:%s] Task remediation `%s`: **%s** after `%s`. %s", util.UUIDToString(claim.ID), p.RemediationKey, strings.ReplaceAll(p.Action, "_", "-"), failureReason, p.Reason)
	comment, err := q.CreateComment(ctx, db.CreateCommentParams{
		IssueID: p.IssueID, WorkspaceID: p.WorkspaceID, AuthorType: "member",
		AuthorID: p.CreatedBy, Content: marker, Type: "system",
	})
	if err != nil {
		return result, fmt.Errorf("create remediation marker: %w", err)
	}
	result.Comment = &comment
	result.Remediation = claim

	if err := tx.Commit(ctx); err != nil {
		return RemediateTaskFailureResult{}, fmt.Errorf("commit remediation: %w", err)
	}

	// Side effects are deliberately post-commit: observers can always reload the
	// rows referenced by an event, and a daemon can never claim an uncommitted task.
	if result.CreatedTask != nil && s.Bus != nil {
		s.broadcastTaskEvent(ctx, protocol.EventTaskQueued, *result.CreatedTask)
	}
	if result.CreatedTask != nil {
		s.notifyTaskAvailable(*result.CreatedTask)
	}
	if s.Bus != nil && (p.Action == RemediationActionBlock || p.Action == RemediationActionReassignRerun) {
		s.broadcastIssueUpdated(result.Issue)
	}
	if p.Action == RemediationActionBlock && originalIssueStatus != result.Issue.Status && s.WebhookService != nil {
		_ = s.WebhookService.RecordIssueStatusTransition(ctx, result.Issue, originalIssueStatus, StatusTransitionOptions{
			Source: "task_remediation", Actor: StatusTransitionActor{Type: "member", ID: p.CreatedBy},
			TaskID: p.FailedTaskID, Metadata: map[string]any{"remediation_id": util.UUIDToString(claim.ID)},
		})
	}
	if s.Bus != nil {
		s.Bus.Publish(events.Event{Type: protocol.EventCommentCreated, WorkspaceID: util.UUIDToString(p.WorkspaceID), ActorType: "member", ActorID: util.UUIDToString(p.CreatedBy), Payload: map[string]any{"comment": comment}})
	}
	return result, nil
}
