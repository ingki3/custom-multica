"use client";

import { useEffect, useState } from "react";
import { Loader2, Lock, Plus, Save, Trash2 } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { toast } from "sonner";

interface McpServer {
  transport?: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
}

interface McpConfig {
  mcpServers?: Record<string, McpServer>;
}

let nextId = 0;

interface ServerEntry {
  id: number;
  name: string;
  transport: string;
  command: string;
  args: string;
  url: string;
  env: string;
}

function configToEntries(config: McpConfig | null): ServerEntry[] {
  if (!config?.mcpServers) return [];
  return Object.entries(config.mcpServers).map(([name, server]) => ({
    id: nextId++,
    name,
    transport: server.transport || "stdio",
    command: server.command || "",
    args: server.args?.join(" ") || "",
    url: server.url || "",
    env: Object.keys(server.env || {}).length > 0 ? JSON.stringify(server.env) : "",
  }));
}

function entriesToConfig(entries: ServerEntry[]): McpConfig | null {
  const valid = entries.filter((e) => e.name.trim());
  if (valid.length === 0) return null;
  const mcpServers: Record<string, McpServer> = {};
  for (const entry of valid) {
    const server: McpServer = { transport: entry.transport };
    if (entry.transport === "stdio") {
      if (entry.command.trim()) server.command = entry.command.trim();
      if (entry.args.trim()) server.args = entry.args.trim().split(/\s+/);
    } else if (entry.transport === "sse" || entry.transport === "streamable-http") {
      if (entry.url.trim()) server.url = entry.url.trim();
    }
    if (entry.env.trim()) {
      try {
        server.env = JSON.parse(entry.env);
      } catch {
        // ignore invalid JSON
      }
    }
    mcpServers[entry.name.trim()] = server;
  }
  return { mcpServers };
}

export function McpConfigTab({
  agent,
  readOnly = false,
  onSave,
  onDirtyChange,
}: {
  agent: Agent;
  readOnly?: boolean;
  onSave: (updates: Partial<Agent>) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const [entries, setEntries] = useState<ServerEntry[]>(
    configToEntries((agent.mcp_config as McpConfig) ?? null),
  );
  const [saving, setSaving] = useState(false);

  const currentJson = JSON.stringify(entriesToConfig(entries));
  const originalJson = JSON.stringify(agent.mcp_config ?? null);
  const dirty = currentJson !== originalJson;

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  const addServer = () => {
    setEntries([
      ...entries,
      { id: nextId++, name: "", transport: "stdio", command: "", args: "", url: "", env: "" },
    ]);
  };

  const removeServer = (index: number) => {
    setEntries(entries.filter((_, i) => i !== index));
  };

  const updateEntry = (index: number, field: keyof ServerEntry, value: string) => {
    setEntries(entries.map((e, i) => (i === index ? { ...e, [field]: value } : e)));
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      const config = entriesToConfig(entries);
      await onSave({ mcp_config: config } as Partial<Agent>);
      toast.success("MCP configuration saved");
    } catch {
      toast.error("Failed to save MCP configuration");
    } finally {
      setSaving(false);
    }
  };

  if (readOnly) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-16 text-muted-foreground">
        <Lock className="h-5 w-5" />
        <p className="text-sm">
          MCP configuration is hidden. Only the agent owner or workspace admins can view it.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium">MCP Servers</h3>
          <p className="text-xs text-muted-foreground">
            Configure Model Context Protocol servers for this agent.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={addServer}>
          <Plus className="h-3.5 w-3.5" />
          Add server
        </Button>
      </div>

      {entries.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No MCP servers configured. Add one to give this agent access to external tools.
          </p>
        </div>
      ) : (
        <div className="space-y-4">
          {entries.map((entry, index) => (
            <div
              key={entry.id}
              className="rounded-lg border p-4 space-y-3"
            >
              <div className="flex items-center gap-2">
                <Input
                  value={entry.name}
                  onChange={(e) => updateEntry(index, "name", e.target.value)}
                  placeholder="Server name"
                  className="flex-1 text-sm"
                />
                <select
                  value={entry.transport}
                  onChange={(e) => updateEntry(index, "transport", e.target.value)}
                  className="h-9 rounded-md border bg-transparent px-3 text-sm"
                >
                  <option value="stdio">stdio</option>
                  <option value="sse">sse</option>
                  <option value="streamable-http">streamable-http</option>
                </select>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => removeServer(index)}
                  className="text-muted-foreground hover:text-destructive"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>

              {entry.transport === "stdio" ? (
                <>
                  <div>
                    <label className="text-xs text-muted-foreground mb-1 block">Command</label>
                    <Input
                      value={entry.command}
                      onChange={(e) => updateEntry(index, "command", e.target.value)}
                      placeholder="/path/to/mcp-server"
                      className="text-sm font-mono"
                    />
                  </div>
                  <div>
                    <label className="text-xs text-muted-foreground mb-1 block">Arguments (space-separated)</label>
                    <Input
                      value={entry.args}
                      onChange={(e) => updateEntry(index, "args", e.target.value)}
                      placeholder="--app desktop"
                      className="text-sm font-mono"
                    />
                  </div>
                </>
              ) : (
                <div>
                  <label className="text-xs text-muted-foreground mb-1 block">URL</label>
                  <Input
                    value={entry.url}
                    onChange={(e) => updateEntry(index, "url", e.target.value)}
                    placeholder="http://localhost:3001/sse"
                    className="text-sm font-mono"
                  />
                </div>
              )}

              <div>
                <label className="text-xs text-muted-foreground mb-1 block">
                  Environment variables (JSON, optional)
                </label>
                <Input
                  value={entry.env}
                  onChange={(e) => updateEntry(index, "env", e.target.value)}
                  placeholder='{"API_KEY": "..."}'
                  className="text-sm font-mono"
                />
              </div>
            </div>
          ))}
        </div>
      )}

      {dirty && (
        <div className="flex items-center justify-end gap-2 pt-2">
          <span className="text-xs text-muted-foreground">Unsaved changes</span>
          <Button size="sm" onClick={handleSave} disabled={saving}>
            {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
            Save
          </Button>
        </div>
      )}
    </div>
  );
}
