-- Workspace-level MCP server registry (2-tier model like skills)
CREATE TABLE workspace_mcp_server (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    transport TEXT NOT NULL DEFAULT 'stdio',
    command TEXT,
    args JSONB DEFAULT '[]',
    url TEXT,
    env JSONB DEFAULT '{}',
    created_by UUID REFERENCES "user"(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(workspace_id, name)
);

-- Agent-to-MCP-server association (many-to-many join)
CREATE TABLE agent_mcp_server (
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    mcp_server_id UUID NOT NULL REFERENCES workspace_mcp_server(id) ON DELETE CASCADE,
    env_override JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (agent_id, mcp_server_id)
);
