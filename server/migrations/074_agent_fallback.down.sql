ALTER TABLE agent DROP CONSTRAINT IF EXISTS agent_fallback_not_self;
ALTER TABLE agent
DROP COLUMN IF EXISTS fallback_max_depth,
DROP COLUMN IF EXISTS fallback_failure_reasons,
DROP COLUMN IF EXISTS fallback_agent_id;
