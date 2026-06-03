-- Issue status transition audit log

-- name: CreateIssueStatusTransition :one
INSERT INTO issue_status_transition (
    workspace_id, issue_id, from_status, to_status, source,
    actor_type, actor_id, task_id, metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: GetIssueStatusTransition :one
SELECT * FROM issue_status_transition
WHERE id = $1;

-- Workspace webhook CRUD

-- name: CreateWorkspaceWebhook :one
INSERT INTO workspace_webhook (
    workspace_id, name, url, secret, enabled, events, filters, created_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: UpdateWorkspaceWebhook :one
UPDATE workspace_webhook SET
    name = COALESCE(sqlc.narg('name'), name),
    url = COALESCE(sqlc.narg('url'), url),
    enabled = COALESCE(sqlc.narg('enabled'), enabled),
    events = COALESCE(sqlc.narg('events'), events),
    filters = COALESCE(sqlc.narg('filters'), filters),
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: RotateWorkspaceWebhookSecret :one
UPDATE workspace_webhook SET
    secret = $3,
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING *;

-- name: DeleteWorkspaceWebhook :exec
DELETE FROM workspace_webhook
WHERE id = $1 AND workspace_id = $2;

-- name: GetWorkspaceWebhook :one
SELECT * FROM workspace_webhook
WHERE id = $1 AND workspace_id = $2;

-- name: ListWorkspaceWebhooks :many
SELECT * FROM workspace_webhook
WHERE workspace_id = $1
ORDER BY created_at DESC;

-- name: ListEnabledWorkspaceWebhooks :many
SELECT * FROM workspace_webhook
WHERE workspace_id = $1 AND enabled = true
ORDER BY created_at ASC;

-- Webhook delivery queue

-- name: CreateWebhookDelivery :one
INSERT INTO webhook_delivery (
    webhook_id, workspace_id, event_type, event_id, payload
) VALUES (
    $1, $2, $3, $4, $5
)
ON CONFLICT (webhook_id, event_id) DO UPDATE SET
    payload = EXCLUDED.payload
RETURNING *;

-- name: GetWebhookDeliveryInWorkspace :one
SELECT * FROM webhook_delivery
WHERE id = $1 AND workspace_id = $2;

-- name: ListWebhookDeliveries :many
SELECT * FROM webhook_delivery
WHERE webhook_id = $1 AND workspace_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ListDueWebhookDeliveries :many
SELECT * FROM webhook_delivery
WHERE status IN ('pending', 'retrying')
  AND next_attempt_at <= now()
ORDER BY next_attempt_at ASC, created_at ASC
LIMIT $1;

-- name: MarkWebhookDeliveryDelivered :one
UPDATE webhook_delivery SET
    status = 'delivered',
    attempt_count = attempt_count + 1,
    last_attempt_at = now(),
    response_status = $2,
    response_body = $3,
    error = NULL,
    delivered_at = now()
WHERE id = $1
RETURNING *;

-- name: MarkWebhookDeliveryRetrying :one
UPDATE webhook_delivery SET
    status = 'retrying',
    attempt_count = attempt_count + 1,
    last_attempt_at = now(),
    response_status = $2,
    response_body = $3,
    error = $4,
    next_attempt_at = $5
WHERE id = $1
RETURNING *;

-- name: MarkWebhookDeliveryFailed :one
UPDATE webhook_delivery SET
    status = 'failed',
    attempt_count = attempt_count + 1,
    last_attempt_at = now(),
    response_status = $2,
    response_body = $3,
    error = $4
WHERE id = $1
RETURNING *;

-- name: RetryWebhookDelivery :one
UPDATE webhook_delivery SET
    status = 'pending',
    next_attempt_at = now(),
    error = NULL
WHERE id = $1 AND workspace_id = $2
RETURNING *;
