-- task_token stores short-lived task-scoped auth credentials minted when
-- a daemon claims a task. The daemon injects the token into the spawned
-- agent process as MULTICA_TOKEN. Requests authenticated by this token are
-- treated as actor=agent/task, not as the daemon owner's full PAT.
CREATE TABLE task_token (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash TEXT NOT NULL,
    task_id UUID NOT NULL REFERENCES agent_task_queue(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_task_token_hash ON task_token(token_hash);
CREATE INDEX idx_task_token_task ON task_token(task_id);
CREATE INDEX idx_task_token_workspace ON task_token(workspace_id);
