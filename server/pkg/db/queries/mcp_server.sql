-- Workspace MCP Server CRUD

-- name: CreateMcpServer :one
INSERT INTO workspace_mcp_server (workspace_id, name, transport, command, args, url, env, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateMcpServer :one
UPDATE workspace_mcp_server SET
    name = COALESCE(sqlc.narg('name'), name),
    transport = COALESCE(sqlc.narg('transport'), transport),
    command = sqlc.narg('command'),
    args = COALESCE(sqlc.narg('args'), args),
    url = sqlc.narg('url'),
    env = COALESCE(sqlc.narg('env'), env),
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteMcpServer :exec
DELETE FROM workspace_mcp_server WHERE id = $1;

-- name: ListMcpServers :many
SELECT * FROM workspace_mcp_server
WHERE workspace_id = $1
ORDER BY name;

-- name: GetMcpServer :one
SELECT * FROM workspace_mcp_server WHERE id = $1;

-- Agent MCP Server association

-- name: AddAgentMcpServer :exec
INSERT INTO agent_mcp_server (agent_id, mcp_server_id, env_override)
VALUES ($1, $2, $3)
ON CONFLICT (agent_id, mcp_server_id) DO UPDATE SET env_override = $3;

-- name: RemoveAgentMcpServer :exec
DELETE FROM agent_mcp_server
WHERE agent_id = $1 AND mcp_server_id = $2;

-- name: RemoveAllAgentMcpServers :exec
DELETE FROM agent_mcp_server WHERE agent_id = $1;

-- name: ListAgentMcpServers :many
SELECT s.*, a.env_override
FROM workspace_mcp_server s
JOIN agent_mcp_server a ON a.mcp_server_id = s.id
WHERE a.agent_id = $1
ORDER BY s.name;

-- name: ListAgentSharedMcpServers :many
-- Used by ClaimTask to merge shared MCP servers with agent-level config
SELECT s.name, s.transport, s.command, s.args, s.url, s.env, a.env_override
FROM workspace_mcp_server s
JOIN agent_mcp_server a ON a.mcp_server_id = s.id
WHERE a.agent_id = $1
ORDER BY s.name;
