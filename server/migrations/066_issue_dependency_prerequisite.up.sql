-- Migrate issue_dependency from unused blocks/blocked_by/related types
-- to a single 'prerequisite' type for execution ordering.
ALTER TABLE issue_dependency DROP CONSTRAINT IF EXISTS issue_dependency_type_check;
ALTER TABLE issue_dependency ADD CONSTRAINT issue_dependency_type_check CHECK (type IN ('prerequisite'));
ALTER TABLE issue_dependency ADD CONSTRAINT issue_dependency_unique UNIQUE (issue_id, depends_on_issue_id);
CREATE INDEX IF NOT EXISTS idx_issue_dependency_issue ON issue_dependency(issue_id);
CREATE INDEX IF NOT EXISTS idx_issue_dependency_depends_on ON issue_dependency(depends_on_issue_id);
