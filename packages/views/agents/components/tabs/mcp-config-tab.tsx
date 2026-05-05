"use client";

import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Loader2, Lock, Save } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { api } from "@multica/core/api";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { toast } from "sonner";

export function McpConfigTab({
  agent,
  readOnly = false,
  onDirtyChange,
}: {
  agent: Agent;
  readOnly?: boolean;
  onSave: (updates: Partial<Agent>) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const wsId = useWorkspaceId();
  const qc = useQueryClient();

  const { data: workspaceServers = [] } = useQuery({
    queryKey: ["mcp-servers", wsId],
    queryFn: () => api.listMcpServers(),
  });
  const { data: agentSharedServers = [] } = useQuery({
    queryKey: ["agent-mcp-servers", agent.id],
    queryFn: () => api.listAgentMcpServers(agent.id),
  });
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setSelectedIds(new Set(agentSharedServers.map((s) => s.id)));
    setDirty(false);
  }, [agentSharedServers]);

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  const toggleServer = (id: string) => {
    const next = new Set(selectedIds);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    setSelectedIds(next);
    setDirty(true);
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await api.setAgentMcpServers(agent.id, {
        servers: Array.from(selectedIds).map((id) => ({ mcp_server_id: id })),
      });
      qc.invalidateQueries({ queryKey: ["agent-mcp-servers", agent.id] });
      setDirty(false);
      toast.success("MCP servers updated");
    } catch {
      toast.error("Failed to update MCP servers");
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
      <div>
        <h3 className="text-sm font-medium">MCP Servers</h3>
        <p className="text-xs text-muted-foreground">
          Select workspace MCP servers for this agent. Manage servers in Settings → MCP Servers.
        </p>
      </div>

      {workspaceServers.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">
            No MCP servers registered in this workspace. Go to Settings → MCP Servers to add one.
          </p>
        </div>
      ) : (
        <div className="space-y-1">
          {workspaceServers.map((server) => {
            const checked = selectedIds.has(server.id);
            return (
              <label
                key={server.id}
                className="flex items-center gap-3 rounded-md border px-3 py-2.5 cursor-pointer hover:bg-accent/50 transition-colors"
              >
                <Checkbox
                  checked={checked}
                  onCheckedChange={() => toggleServer(server.id)}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{server.name}</span>
                    <span className="rounded-full bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                      {server.transport}
                    </span>
                  </div>
                  <p className="text-xs text-muted-foreground truncate font-mono">
                    {server.transport === "stdio"
                      ? [server.command, ...(server.args ?? [])].filter(Boolean).join(" ")
                      : server.url ?? ""}
                  </p>
                </div>
                {checked && <Check className="h-3.5 w-3.5 text-muted-foreground shrink-0" />}
              </label>
            );
          })}
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
