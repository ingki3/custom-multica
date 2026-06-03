CREATE TABLE issue_status_transition (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    from_status TEXT NOT NULL,
    to_status TEXT NOT NULL,
    source TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id UUID,
    task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_issue_status_transition_workspace_created
    ON issue_status_transition(workspace_id, created_at DESC);
CREATE INDEX idx_issue_status_transition_issue_created
    ON issue_status_transition(issue_id, created_at DESC);

CREATE TABLE workspace_webhook (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    secret TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    events JSONB NOT NULL DEFAULT '["issue.status_changed"]'::jsonb,
    filters JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_webhook_workspace
    ON workspace_webhook(workspace_id);

CREATE TABLE webhook_delivery (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    webhook_id UUID NOT NULL REFERENCES workspace_webhook(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    event_id UUID NOT NULL,
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempt_count INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_attempt_at TIMESTAMPTZ,
    response_status INT,
    response_body TEXT,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ,
    UNIQUE(webhook_id, event_id)
);

CREATE INDEX idx_webhook_delivery_pending
    ON webhook_delivery(status, next_attempt_at)
    WHERE status IN ('pending', 'retrying');
CREATE INDEX idx_webhook_delivery_webhook_created
    ON webhook_delivery(webhook_id, created_at DESC);
