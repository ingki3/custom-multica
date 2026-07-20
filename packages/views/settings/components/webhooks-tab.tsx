"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Pencil, Plus, RotateCw, Send, Trash2 } from "lucide-react";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import type { WebhookDelivery, WebhookEvent, WebhookFilters, WorkspaceWebhook } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Dialog, DialogContent, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Switch } from "@multica/ui/components/ui/switch";
import { toast } from "sonner";
import {
  WEBHOOK_EVENT_OPTIONS,
  WEBHOOK_FAILURE_REASON_VALUES,
  WEBHOOK_SOURCE_VALUES,
  WEBHOOK_STATUS_VALUES,
  invalidWebhookFilterValues,
  normalizeWebhookConfig,
  parseWebhookFilterCsv,
} from "./webhook-config";

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
          <p className="text-sm text-muted-foreground">Send workspace and operational events to external automation.</p>
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
                <button
                  className="min-w-0 flex-1 text-left"
                  onClick={() => setSelected(webhook)}
                  aria-label={`View deliveries for ${webhook.name}`}
                >
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{webhook.name}</span>
                    <span className="rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
                      {webhook.enabled ? "Enabled" : "Disabled"}
                    </span>
                  </div>
                  <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">{webhook.url}</p>
                  <div className="mt-2 flex flex-wrap gap-1">
                    {webhook.events.map((event) => (
                      <Badge key={event} variant="outline">{event}</Badge>
                    ))}
                  </div>
                </button>
                <div className="flex shrink-0 items-center gap-1">
                  <Button variant="ghost" size="icon-sm" onClick={() => handleTest(webhook.id)} aria-label={`Test ${webhook.name}`}>
                    <Send className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" onClick={() => handleRotate(webhook.id)} aria-label={`Rotate secret for ${webhook.name}`}>
                    <RotateCw className="h-3.5 w-3.5" />
                  </Button>
                  <Button variant="ghost" size="icon-sm" onClick={() => setEditing(webhook)} aria-label={`Edit ${webhook.name}`}>
                    <Pencil className="h-3.5 w-3.5" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="text-muted-foreground hover:text-destructive"
                    onClick={() => handleDelete(webhook.id)}
                    aria-label={`Delete ${webhook.name}`}
                  >
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
                  <Badge variant="secondary">{d.event_type}</Badge>
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
  const [events, setEvents] = useState<WebhookEvent[]>(webhook?.events ?? ["issue.status_changed"]);
  const [from, setFrom] = useState((webhook?.filters.status_from ?? []).join(", "));
  const [to, setTo] = useState((webhook?.filters.status_to ?? []).join(", "));
  const [source, setSource] = useState((webhook?.filters.source ?? []).join(", "));
  const [failureReason, setFailureReason] = useState((webhook?.filters.failure_reason ?? []).join(", "));
  const [willRetry, setWillRetry] = useState<"" | "true" | "false">(webhook?.filters.will_retry?.[0] ?? "");
  const [saving, setSaving] = useState(false);

  const toggleEvent = (event: WebhookEvent) => {
    setEvents((current) => current.includes(event)
      ? current.filter((value) => value !== event)
      : [...current, event]);
  };

  const handleSave = async () => {
    if (!name.trim() || !url.trim()) return;
    const invalidFilters = [
      ...(events.includes("issue.status_changed") ? [
        ...invalidWebhookFilterValues(from, WEBHOOK_STATUS_VALUES),
        ...invalidWebhookFilterValues(to, WEBHOOK_STATUS_VALUES),
        ...invalidWebhookFilterValues(source, WEBHOOK_SOURCE_VALUES),
      ] : []),
      ...(events.includes("task.failed")
        ? invalidWebhookFilterValues(failureReason, WEBHOOK_FAILURE_REASON_VALUES)
        : []),
    ];
    if (invalidFilters.length > 0) {
      const uniqueInvalidFilters = [...new Set(invalidFilters)];
      toast.error(`Unsupported filter value${uniqueInvalidFilters.length === 1 ? "" : "s"}: ${uniqueInvalidFilters.join(", ")}`);
      return;
    }
    setSaving(true);
    try {
      const candidateFilters: WebhookFilters = {
        status_from: parseWebhookFilterCsv(from, WEBHOOK_STATUS_VALUES),
        status_to: parseWebhookFilterCsv(to, WEBHOOK_STATUS_VALUES),
        source: parseWebhookFilterCsv(source, WEBHOOK_SOURCE_VALUES),
        failure_reason: parseWebhookFilterCsv(failureReason, WEBHOOK_FAILURE_REASON_VALUES),
        will_retry: willRetry ? [willRetry] : undefined,
      };
      const config = normalizeWebhookConfig(events, candidateFilters);
      if (isEdit) {
        await api.updateWebhook(webhook.id, { name: name.trim(), url: url.trim(), enabled, ...config });
        toast.success("Webhook updated");
        onSaved();
      } else {
        const created = await api.createWebhook({ name: name.trim(), url: url.trim(), enabled, ...config });
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
      <DialogContent className="max-h-[85vh] max-w-lg overflow-y-auto">
        <DialogTitle>{isEdit ? "Edit Webhook" : "Add Webhook"}</DialogTitle>
        <div className="space-y-4 pt-2">
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name" aria-label="Webhook name" />
          <Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://example.com/webhook" className="font-mono text-sm" aria-label="Webhook URL" />
          <div className="flex items-center justify-between rounded-md border p-3">
            <label htmlFor="webhook-enabled" className="text-sm">Enabled</label>
            <Switch id="webhook-enabled" checked={enabled} onCheckedChange={setEnabled} />
          </div>
          <fieldset className="space-y-2">
            <legend className="mb-2 text-sm font-medium">Events</legend>
            {WEBHOOK_EVENT_OPTIONS.map((option) => (
              <label key={option.value} className="flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors hover:bg-accent/50">
                <Checkbox
                  checked={events.includes(option.value)}
                  onCheckedChange={() => toggleEvent(option.value)}
                  aria-label={option.label}
                />
                <span className="min-w-0">
                  <span className="block text-sm font-medium">{option.label}</span>
                  <span className="block text-xs text-muted-foreground">{option.description}</span>
                </span>
              </label>
            ))}
            {events.length === 0 ? <p className="text-xs text-destructive">Select at least one event.</p> : null}
          </fieldset>
          {events.includes("issue.status_changed") ? (
            <fieldset className="space-y-2">
              <legend className="mb-2 text-sm font-medium">Issue event filters</legend>
              <Input value={from} onChange={(e) => setFrom(e.target.value)} placeholder="From statuses, comma-separated" aria-label="From statuses" />
              <Input value={to} onChange={(e) => setTo(e.target.value)} placeholder="To statuses, comma-separated" aria-label="To statuses" />
              <Input value={source} onChange={(e) => setSource(e.target.value)} placeholder="Sources, comma-separated" aria-label="Sources" />
              <p className="text-xs text-muted-foreground">Leave a filter empty to receive all matching issue events.</p>
            </fieldset>
          ) : null}
          {events.includes("task.failed") ? (
            <fieldset className="space-y-2">
              <legend className="mb-2 text-sm font-medium">Task failure filters</legend>
              <Input
                value={failureReason}
                onChange={(e) => setFailureReason(e.target.value)}
                placeholder="Failure reasons, comma-separated"
                aria-label="Failure reasons"
              />
              <NativeSelect
                className="w-full"
                value={willRetry}
                onChange={(event) => setWillRetry(event.target.value as "" | "true" | "false")}
                aria-label="Will retry"
              >
                <NativeSelectOption value="">Any retry state</NativeSelectOption>
                <NativeSelectOption value="true">Will retry</NativeSelectOption>
                <NativeSelectOption value="false">Will not retry</NativeSelectOption>
              </NativeSelect>
              <p className="text-xs text-muted-foreground">Leave a filter empty to receive all task failures.</p>
            </fieldset>
          ) : null}
          <div className="flex justify-end gap-2 pt-2">
            <Button variant="outline" size="sm" onClick={onClose}>Cancel</Button>
            <Button size="sm" onClick={handleSave} disabled={!name.trim() || !url.trim() || events.length === 0 || saving}>
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
              {isEdit ? "Save" : "Create"}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
