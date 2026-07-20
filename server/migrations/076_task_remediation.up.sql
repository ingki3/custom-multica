CREATE TABLE task_remediation (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    issue_id UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    failed_task_id UUID NOT NULL REFERENCES agent_task_queue(id) ON DELETE CASCADE,
    source_agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    target_agent_id UUID REFERENCES agent(id) ON DELETE SET NULL,
    created_task_id UUID REFERENCES agent_task_queue(id) ON DELETE SET NULL,
    remediation_key TEXT NOT NULL CHECK (char_length(remediation_key) BETWEEN 1 AND 200),
    action TEXT NOT NULL CHECK (action IN ('rerun', 'reassign_rerun', 'block')),
    automated BOOLEAN NOT NULL DEFAULT true,
    reason TEXT NOT NULL CHECK (char_length(reason) BETWEEN 1 AND 500),
    created_by UUID REFERENCES "user"(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, remediation_key),
    UNIQUE (failed_task_id)
);

CREATE UNIQUE INDEX task_remediation_one_automated_per_issue
    ON task_remediation(issue_id) WHERE automated;
CREATE INDEX task_remediation_workspace_created
    ON task_remediation(workspace_id, created_at DESC);
