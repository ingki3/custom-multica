"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Trash2, Loader2, Pencil } from "lucide-react";
import { useWorkspaceId } from "@multica/core/hooks";
import { api } from "@multica/core/api";
import type { McpServer } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Dialog, DialogContent, DialogTitle } from "@multica/ui/components/ui/dialog";
import { toast } from "sonner";

function mcpServerQueryKey(wsId: string) {
  return ["mcp-servers", wsId] as const;
}

export function McpServersTab() {
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const { data: servers = [], isLoading } = useQuery({
    queryKey: mcpServerQueryKey(wsId),
    queryFn: () => api.listMcpServers(),
  });
  const [editingServer, setEditingServer] = useState<McpServer | null>(null);
  const [creating, setCreating] = useState(false);

  const invalidate = () => qc.invalidateQueries({ queryKey: mcpServerQueryKey(wsId) });

  const handleDelete = async (id: string) => {
    await api.deleteMcpServer(id);
    invalidate();
    toast.success("MCP server deleted");
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">MCP Servers</h2>
          <p className="text-sm text-muted-foreground">
            Register MCP servers at the workspace level. Agents can select which servers to use.
          </p>
        </div>
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus className="h-3.5 w-3.5" />
          Add server
        </Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : servers.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No MCP servers registered. Add one to make it available to agents in this workspace.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {servers.map((server) => (
            <div
              key={server.id}
              className="flex items-center justify-between rounded-lg border p-4"
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="font-medium text-sm">{server.name}</span>
                  <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                    {server.transport}
                  </span>
                </div>
                <p className="text-xs text-muted-foreground truncate mt-0.5 font-mono">
                  {server.transport === "stdio"
                    ? [server.command, ...(server.args ?? [])].filter(Boolean).join(" ")
                    : server.url ?? ""}
                </p>
              </div>
              <div className="flex items-center gap-1 shrink-0 ml-3">
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => setEditingServer(server)}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => handleDelete(server.id)}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      {(creating || editingServer) && (
        <McpServerDialog
          server={editingServer}
          onClose={() => { setCreating(false); setEditingServer(null); }}
          onSaved={() => { setCreating(false); setEditingServer(null); invalidate(); }}
        />
      )}
    </div>
  );
}

function McpServerDialog({
  server,
  onClose,
  onSaved,
}: {
  server: McpServer | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const isEdit = !!server;
  const [name, setName] = useState(server?.name ?? "");
  const [transport, setTransport] = useState(server?.transport ?? "stdio");
  const [command, setCommand] = useState(server?.command ?? "");
  const [args, setArgs] = useState(server?.args?.join(" ") ?? "");
  const [url, setUrl] = useState(server?.url ?? "");
  const [env, setEnv] = useState(
    server?.env && Object.keys(server.env).length > 0 ? JSON.stringify(server.env) : "",
  );
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    if (!name.trim()) return;
    setSaving(true);
    try {
      let envObj: Record<string, string> | undefined;
      if (env.trim()) {
        try { envObj = JSON.parse(env); } catch { /* ignore */ }
      }

      const data = {
        name: name.trim(),
        transport,
        command: transport === "stdio" ? command.trim() || undefined : undefined,
        args: transport === "stdio" && args.trim() ? args.trim().split(/\s+/) : undefined,
        url: transport !== "stdio" ? url.trim() || undefined : undefined,
        env: envObj,
      };

      if (isEdit) {
        await api.updateMcpServer(server.id, data);
        toast.success("MCP server updated");
      } else {
        await api.createMcpServer(data);
        toast.success("MCP server created");
      }
      onSaved();
    } catch {
      toast.error(isEdit ? "Failed to update" : "Failed to create");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-md">
        <DialogTitle>{isEdit ? "Edit MCP Server" : "Add MCP Server"}</DialogTitle>
        <div className="space-y-4 pt-2">
          <div className="flex items-center gap-2">
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Server name"
              className="flex-1"
            />
            <select
              value={transport}
              onChange={(e) => setTransport(e.target.value)}
              className="h-8 rounded-md border bg-transparent px-3 text-sm"
            >
              <option value="stdio">stdio</option>
              <option value="sse">sse</option>
              <option value="streamable-http">streamable-http</option>
            </select>
          </div>

          {transport === "stdio" ? (
            <>
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">Command</label>
                <Input
                  value={command}
                  onChange={(e) => setCommand(e.target.value)}
                  placeholder="/path/to/mcp-server"
                  className="font-mono text-sm"
                />
              </div>
              <div>
                <label className="text-xs text-muted-foreground mb-1 block">Arguments (space-separated)</label>
                <Input
                  value={args}
                  onChange={(e) => setArgs(e.target.value)}
                  placeholder="--app desktop"
                  className="font-mono text-sm"
                />
              </div>
            </>
          ) : (
            <div>
              <label className="text-xs text-muted-foreground mb-1 block">URL</label>
              <Input
                value={url}
                onChange={(e) => setUrl(e.target.value)}
                placeholder="http://localhost:3001/sse"
                className="font-mono text-sm"
              />
            </div>
          )}

          <div>
            <label className="text-xs text-muted-foreground mb-1 block">
              Environment variables (JSON, optional)
            </label>
            <Input
              value={env}
              onChange={(e) => setEnv(e.target.value)}
              placeholder='{"API_KEY": "..."}'
              className="font-mono text-sm"
            />
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={onClose}>Cancel</Button>
            <Button size="sm" onClick={handleSave} disabled={!name.trim() || saving}>
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
              {isEdit ? "Save" : "Create"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
