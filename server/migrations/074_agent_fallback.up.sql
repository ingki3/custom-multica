ALTER TABLE agent
ADD COLUMN fallback_agent_id uuid REFERENCES agent(id) ON DELETE SET NULL,
ADD COLUMN fallback_failure_reasons text[] NOT NULL DEFAULT ARRAY['context_limit', 'rate_limit', 'model_limit']::text[],
ADD COLUMN fallback_max_depth integer NOT NULL DEFAULT 1;

ALTER TABLE agent
ADD CONSTRAINT agent_fallback_not_self
CHECK (fallback_agent_id IS NULL OR fallback_agent_id <> id);
