package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/redact"
)

const taskFailureErrorMaxRunes = 500

func newFailureDisposition(task db.AgentTaskQueue, retryTask *db.AgentTaskQueue, retryKind RetryKind) FailureDisposition {
	if retryTask == nil {
		retryKind = RetryKindNone
	}
	return FailureDisposition{
		Task: task, RetryTask: retryTask, RetryKind: retryKind,
		WillRetry: retryTask != nil, FinalFailure: retryTask == nil,
	}
}

func boundedTaskFailureError(raw string) string {
	redacted := redact.Text(raw)
	runes := []rune(redacted)
	if len(runes) > taskFailureErrorMaxRunes {
		return string(runes[:taskFailureErrorMaxRunes])
	}
	return redacted
}

func buildTaskFailureRealtimePayload(d FailureDisposition) map[string]any {
	task := d.Task
	failureReason := "agent_error"
	if task.FailureReason.Valid && strings.TrimSpace(task.FailureReason.String) != "" {
		failureReason = task.FailureReason.String
	}
	payload := map[string]any{
		"task_id":        util.UUIDToString(task.ID),
		"agent_id":       util.UUIDToString(task.AgentID),
		"issue_id":       util.UUIDToString(task.IssueID),
		"status":         task.Status,
		"failure_reason": failureReason,
		"will_retry":     d.WillRetry,
		"final_failure":  d.FinalFailure,
		"retry_kind":     string(d.RetryKind),
		"retry_task_id":  nil,
	}
	if task.ChatSessionID.Valid {
		payload["chat_session_id"] = util.UUIDToString(task.ChatSessionID)
	}
	if d.RetryTask != nil {
		payload["retry_task_id"] = util.UUIDToString(d.RetryTask.ID)
	}
	return payload
}

// BuildTaskFailedPayload builds the public operational webhook envelope. It
// intentionally starts from the persisted task row and a resolved workspace;
// prompts, context, workdirs, sessions, and raw execution output never cross
// this trust boundary.
func (s *TaskService) BuildTaskFailedPayload(ctx context.Context, d FailureDisposition) (map[string]any, bool) {
	task := d.Task
	workspaceID := s.ResolveTaskWorkspaceID(ctx, task)
	if workspaceID == "" {
		// Quick-create and other unscoped tasks must not produce an invalid
		// external envelope merely because they have an agent/runtime.
		return nil, false
	}
	workspaceUUID, err := util.ParseUUID(workspaceID)
	if err != nil {
		return nil, false
	}
	workspace, err := s.Queries.GetWorkspace(ctx, workspaceUUID)
	if err != nil {
		return nil, false
	}

	var issue *db.Issue
	var project *db.Project
	if task.IssueID.Valid {
		if loadedIssue, issueErr := s.Queries.GetIssue(ctx, task.IssueID); issueErr == nil {
			issue = &loadedIssue
			if loadedIssue.ProjectID.Valid {
				if loadedProject, projectErr := s.Queries.GetProject(ctx, loadedIssue.ProjectID); projectErr == nil {
					project = &loadedProject
				}
			}
		}
	}
	return buildTaskFailedPayload(d, workspace, issue, project), true
}

// buildTaskFailedPayload is the side-effect-free serialization boundary used
// after workspace/issue/project rows have been loaded. Keeping this pure makes
// the public contract (including redaction and current persisted issue status)
// directly testable without duplicating the production payload logic.
func buildTaskFailedPayload(d FailureDisposition, workspace db.Workspace, issue *db.Issue, project *db.Project) map[string]any {
	task := d.Task
	occurredAt := time.Now().UTC()
	if task.CompletedAt.Valid {
		occurredAt = task.CompletedAt.Time
	}
	failureReason := "agent_error"
	if task.FailureReason.Valid && strings.TrimSpace(task.FailureReason.String) != "" {
		failureReason = task.FailureReason.String
	}
	errorSummary := ""
	if task.Error.Valid {
		errorSummary = boundedTaskFailureError(task.Error.String)
	}

	taskPayload := map[string]any{
		"id":             util.UUIDToString(task.ID),
		"agent_id":       uuidStringOrNil(task.AgentID),
		"runtime_id":     uuidStringOrNil(task.RuntimeID),
		"attempt":        task.Attempt,
		"max_attempts":   task.MaxAttempts,
		"failure_reason": failureReason,
		"error":          errorSummary,
		"will_retry":     d.WillRetry,
		"retry_kind":     string(d.RetryKind),
		"retry_task_id":  nil,
		"completed_at":   timestampStringOrNil(task.CompletedAt),
	}
	if d.RetryTask != nil {
		taskPayload["retry_task_id"] = util.UUIDToString(d.RetryTask.ID)
	}

	payload := map[string]any{
		"schema_version": 1,
		"event":          EventTaskFailed,
		"event_type":     EventTaskFailed,
		"event_id":       util.UUIDToString(task.ID),
		"occurred_at":    occurredAt.Format(time.RFC3339),
		"workspace": map[string]any{
			"id": util.UUIDToString(workspace.ID), "slug": workspace.Slug, "name": workspace.Name,
		},
		"task": taskPayload,
	}

	if issue != nil {
		prefix := workspace.IssuePrefix
		if prefix == "" {
			prefix = strings.ToUpper(workspace.Slug)
		}
		payload["issue"] = map[string]any{
			"id": util.UUIDToString(issue.ID), "identifier": fmt.Sprintf("%s-%d", prefix, issue.Number),
			"number": issue.Number, "title": issue.Title, "status": issue.Status,
		}
		if project != nil {
			payload["project"] = map[string]any{"id": util.UUIDToString(project.ID), "title": project.Title}
		}
	}
	return payload
}
