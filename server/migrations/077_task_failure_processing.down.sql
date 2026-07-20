DROP INDEX IF EXISTS agent_task_queue_one_child_per_parent;

ALTER TABLE agent_task_queue
    DROP COLUMN IF EXISTS failure_handled_at,
    DROP COLUMN IF EXISTS failure_processing_started_at;
