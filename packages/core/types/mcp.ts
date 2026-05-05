export interface McpServer {
  id: string;
  name: string;
  transport: string;
  command?: string;
  args?: string[];
  url?: string;
  env?: Record<string, string>;
  created_at: string;
  updated_at: string;
}

export interface CreateMcpServerRequest {
  name: string;
  transport: string;
  command?: string;
  args?: string[];
  url?: string;
  env?: Record<string, string>;
}

export interface UpdateMcpServerRequest {
  name?: string;
  transport?: string;
  command?: string;
  args?: string[];
  url?: string;
  env?: Record<string, string>;
}

export interface AgentMcpServerEntry {
  mcp_server_id: string;
  env_override?: Record<string, string>;
}
