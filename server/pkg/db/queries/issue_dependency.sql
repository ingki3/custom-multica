-- name: CreateIssueDependency :one
INSERT INTO issue_dependency (issue_id, depends_on_issue_id, type)
VALUES ($1, $2, 'prerequisite')
RETURNING *;

-- name: DeleteIssueDependency :exec
DELETE FROM issue_dependency WHERE id = $1;

-- name: DeleteIssueDependencyByPair :exec
DELETE FROM issue_dependency
WHERE issue_id = $1 AND depends_on_issue_id = $2;

-- name: ListPrerequisites :many
-- Issues that must be Done before this issue can execute
SELECT d.id, d.issue_id, d.depends_on_issue_id, d.type,
       i.title AS depends_on_title, i.status AS depends_on_status,
       i.number AS depends_on_number
FROM issue_dependency d
JOIN issue i ON i.id = d.depends_on_issue_id
WHERE d.issue_id = $1
ORDER BY i.created_at;

-- name: ListNextIssues :many
-- Issues that become executable when this issue reaches Done
SELECT d.id, d.issue_id, d.depends_on_issue_id, d.type,
       i.title AS next_title, i.status AS next_status,
       i.number AS next_number
FROM issue_dependency d
JOIN issue i ON i.id = d.issue_id
WHERE d.depends_on_issue_id = $1
ORDER BY i.created_at;

-- name: HasUnmetPrerequisites :one
SELECT EXISTS (
    SELECT 1 FROM issue_dependency dep
    JOIN issue i ON i.id = dep.depends_on_issue_id
    WHERE dep.issue_id = $1
      AND i.status != 'done'
) AS has_unmet;

-- name: ListUnblockedNextIssues :many
-- When an issue reaches Done, find its next issues whose ALL prerequisites are now met
SELECT DISTINCT i.* FROM issue i
JOIN issue_dependency dep ON dep.issue_id = i.id AND dep.depends_on_issue_id = $1
WHERE NOT EXISTS (
    SELECT 1 FROM issue_dependency other_dep
    JOIN issue prereq ON prereq.id = other_dep.depends_on_issue_id
    WHERE other_dep.issue_id = i.id
      AND prereq.status != 'done'
);

-- DetectCycle is implemented in Go (handler) using a recursive query
-- because sqlc's parser does not support WITH RECURSIVE inside CASE/subquery.
