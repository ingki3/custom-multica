-- name: LockIssueForRemediation :one
SELECT * FROM issue
WHERE id = @issue_id AND workspace_id = @workspace_id
FOR UPDATE;

-- name: GetTaskRemediationByKey :one
SELECT * FROM task_remediation
WHERE workspace_id = @workspace_id AND remediation_key = @remediation_key;

-- name: GetTaskRemediationByFailedTask :one
SELECT * FROM task_remediation
WHERE failed_task_id = @failed_task_id;

-- name: GetFailedTaskForRemediation :one
SELECT atq.* FROM agent_task_queue atq
JOIN agent a ON a.id = atq.agent_id
WHERE atq.id = @failed_task_id
  AND atq.issue_id = @issue_id
  AND a.workspace_id = @workspace_id
FOR UPDATE OF atq;

-- name: HasActiveTaskForRemediationIssue :one
SELECT EXISTS (
    SELECT 1 FROM agent_task_queue
    WHERE issue_id = @issue_id AND status IN ('queued', 'dispatched', 'running')
) AS has_active;

-- name: HasAutomatedTaskRemediation :one
SELECT EXISTS (
    SELECT 1 FROM task_remediation
    WHERE issue_id = @issue_id AND automated
) AS has_automated;

-- name: CreateTaskRemediationClaim :one
INSERT INTO task_remediation (
    workspace_id, issue_id, failed_task_id, source_agent_id, target_agent_id,
    remediation_key, action, automated, reason, created_by
) VALUES (
    @workspace_id, @issue_id, @failed_task_id, @source_agent_id,
    sqlc.narg(target_agent_id), @remediation_key, @action, @automated,
    @reason, sqlc.narg(created_by)
)
ON CONFLICT (workspace_id, remediation_key) DO NOTHING
RETURNING *;

-- name: CreateTaskRemediationChild :one
INSERT INTO agent_task_queue (
    agent_id, runtime_id, issue_id, chat_session_id, autopilot_run_id,
    status, priority, trigger_comment_id, trigger_summary, context,
    session_id, work_dir, attempt, max_attempts, parent_task_id
)
SELECT
    @target_agent_id, @target_runtime_id, p.issue_id, p.chat_session_id, p.autopilot_run_id,
    'queued', p.priority, p.trigger_comment_id, p.trigger_summary, p.context,
    p.session_id, p.work_dir, p.attempt + 1, p.max_attempts, p.id
FROM agent_task_queue p
WHERE p.id = @failed_task_id AND p.status = 'failed'
RETURNING *;

-- name: SetTaskRemediationCreatedTask :one
UPDATE task_remediation
SET created_task_id = @created_task_id
WHERE id = @id
RETURNING *;

-- name: ReassignIssueForRemediation :one
UPDATE issue
SET assignee_type = 'agent', assignee_id = @target_agent_id, updated_at = now()
WHERE id = @issue_id
RETURNING *;

-- name: BlockIssueForRemediation :one
UPDATE issue
SET status = 'blocked', updated_at = now()
WHERE id = @issue_id
RETURNING *;
