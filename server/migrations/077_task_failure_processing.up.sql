ALTER TABLE agent_task_queue
    ADD COLUMN failure_processing_started_at TIMESTAMPTZ,
    ADD COLUMN failure_handled_at TIMESTAMPTZ;

CREATE UNIQUE INDEX agent_task_queue_one_child_per_parent
    ON agent_task_queue(parent_task_id)
    WHERE parent_task_id IS NOT NULL;
