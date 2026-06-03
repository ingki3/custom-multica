"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, Plus, RotateCw, Send, Trash2 } from "lucide-react";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import type { WebhookDelivery, WebhookFilters, WorkspaceWebhook } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Switch } from "@multica/ui/components/ui/switch";
import { toast } from "sonner";

const STATUS_VALUES = ["backlog", "todo", "in_progress", "in_review", "done", "blocked", "cancelled"];
const SOURCE_VALUES = ["task_started", "task_completed", "task_failed", "dependency_activation", "manual_update", "agent_cli", "batch_update"];

function webhookQueryKey(wsId: string) {
  return ["webhooks", wsId] as const;
}

function deliveryQueryKey(wsId: string, id: string | null) {
  return ["webhooks", wsId, id, "deliveries"] as const;
}

export function WebhooksTab() {
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const [editing, setEditing] = useState<WorkspaceWebhook | null>(null);
  const [creating, setCreating] = useState(false);
  const [selected, setSelected] = useState<WorkspaceWebhook | null>(null);
  const [newSecret, setNewSecret] = useState<string | null>(null);

  const { data: webhooks = [], isLoading } = useQuery({
    queryKey: webhookQueryKey(wsId),
    queryFn: () => api.listWebhooks(),
  });

  const { data: deliveries = [] } = useQuery({
    queryKey: deliveryQueryKey(wsId, selected?.id ?? null),
    queryFn: () => selected ? api.listWebhookDeliveries(selected.id) : Promise.resolve([]),
    enabled: !!selected,
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: webhookQueryKey(wsId) });
  const invalidateDeliveries = (id: string) => qc.invalidateQueries({ queryKey: deliveryQueryKey(wsId, id) });

  const handleDelete = async (id: string) => {
    await api.deleteWebhook(id);
    if (selected?.id === id) setSelected(null);
    invalidate();
    toast.success("Webhook deleted");
  };

  const handleTest = async (id: string) => {
    await api.testWebhook(id);
    invalidateDeliveries(id);
    toast.success("Test delivery queued");
  };

  const handleRotate = async (id: string) => {
    const wh = await api.rotateWebhookSecret(id);
    setNewSecret(wh.secret ?? null);
    invalidate();
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-base font-medium">Webhooks</h2>
          <p className="text-sm text-muted-foreground">Send issue status transitions to external automation.</p>
        </div>
        <Button size="sm" onClick={() => setCreating(true)}>
          <Plus className="h-3.5 w-3.5" />
          Add webhook
        </Button>
      </div>

      {newSecret ? (
        <div className="rounded-md border border-warning/30 bg-warning/10 p-3">
          <p className="text-xs text-muted-foreground mb-1">Webhook secret</p>
          <code className="block truncate text-xs">{newSecret}</code>
        </div>
      ) : null}

      {isLoading ? (
        <div className="flex justify-center py-8">
          <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
        </div>
      ) : webhooks.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <p className="text-sm text-muted-foreground">No webhooks registered.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {webhooks.map((webhook) => (
            <div key={webhook.id} className="rounded-lg border p-4">
              <div className="flex items-center justify-between gap-3">
                <button className="min-w-0 flex-1 text-left" onClick={() => setSelected(webhook)}>
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{webhook.name}</span>
                    <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                      {webhook.enabled ? "Enabled" : "Disabled"}
                    </span>
                  </div>
                  <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">{webhook.url}</p>
                </button>
                <div className="flex shrink-0 items-center gap-1">
                  <Button variant="ghost" size="icon-sm" onClick={() => handleTest(webhook.id)}>
                    <Send className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" onClick={() => handleRotate(webhook.id)}>
                    <RotateCw className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" onClick={() => setEditing(webhook)}>
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-destructive" onClick={() => handleDelete(webhook.id)}>
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {selected ? (
        <DeliveryList
          deliveries={deliveries}
          onRetry={async (id) => {
            await api.retryWebhookDelivery(id);
            invalidateDeliveries(selected.id);
            toast.success("Delivery queued");
          }}
        />
      ) : null}

      {(creating || editing) ? (
        <WebhookDialog
          webhook={editing}
          onClose={() => { setCreating(false); setEditing(null); }}
          onSaved={(secret) => {
            setCreating(false);
            setEditing(null);
            setNewSecret(secret ?? null);
            invalidate();
          }}
        />
      ) : null}
    </div>
  );
}

function DeliveryList({ deliveries, onRetry }: { deliveries: WebhookDelivery[]; onRetry: (id: string) => Promise<void> }) {
  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium">Recent deliveries</h3>
      {deliveries.length === 0 ? (
        <p className="text-sm text-muted-foreground">No deliveries yet.</p>
      ) : (
        <div className="space-y-2">
          {deliveries.map((d) => (
            <div key={d.id} className="flex items-center justify-between rounded-md border p-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{d.status}</span>
                  <span className="text-xs text-muted-foreground">{d.response_status ?? "no response"}</span>
                </div>
                <p className="truncate font-mono text-xs text-muted-foreground">{d.event_id}</p>
                {d.error ? <p className="truncate text-xs text-destructive">{d.error}</p> : null}
              </div>
              {d.status !== "delivered" ? (
                <Button variant="outline" size="sm" onClick={() => onRetry(d.id)}>Retry</Button>
              ) : null}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function WebhookDialog({
  webhook,
  onClose,
  onSaved,
}: {
  webhook: WorkspaceWebhook | null;
  onClose: () => void;
  onSaved: (secret?: string) => void;
}) {
  const isEdit = !!webhook;
  const [name, setName] = useState(webhook?.name ?? "");
  const [url, setUrl] = useState(webhook?.url ?? "");
  const [enabled, setEnabled] = useState(webhook?.enabled ?? true);
  const [from, setFrom] = useState((webhook?.filters.status_from ?? []).join(", "));
  const [to, setTo] = useState((webhook?.filters.status_to ?? []).join(", "));
  const [source, setSource] = useState((webhook?.filters.source ?? []).join(", "));
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    if (!name.trim() || !url.trim()) return;
    setSaving(true);
    try {
      const filters: WebhookFilters = {
        status_from: parseCsv(from, STATUS_VALUES),
        status_to: parseCsv(to, STATUS_VALUES),
        source: parseCsv(source, SOURCE_VALUES),
      };
      if (isEdit) {
        await api.updateWebhook(webhook.id, { name: name.trim(), url: url.trim(), enabled, events: ["issue.status_changed"], filters });
        toast.success("Webhook updated");
        onSaved();
      } else {
        const created = await api.createWebhook({ name: name.trim(), url: url.trim(), enabled, events: ["issue.status_changed"], filters });
        toast.success("Webhook created");
        onSaved(created.secret);
      }
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to save webhook");
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="max-w-md">
        <DialogTitle>{isEdit ? "Edit Webhook" : "Add Webhook"}</DialogTitle>
        <div className="space-y-4 pt-2">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" />
          <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com/webhook" className="font-mono text-sm" />
          <div className="flex items-center justify-between rounded-md border p-3">
            <span className="text-sm">Enabled</span>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </div>
          <div className="space-y-2">
            <Input value={from} onChange={(e) => setFrom(e.target.value)} placeholder="From statuses, comma-separated" />
            <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="To statuses, comma-separated" />
            <Input value={source} onChange={(e) => setSource(e.target.value)} placeholder="Sources, comma-separated" />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={onClose}>Cancel</Button>
            <Button size="sm" onClick={handleSave} disabled={!name.trim() || !url.trim() || saving}>
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
              {isEdit ? "Save" : "Create"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function parseCsv(value: string, allowed: string[]) {
  const items = value.split(",").map((x) => x.trim()).filter(Boolean);
  return items.filter((x) => allowed.includes(x));
}
