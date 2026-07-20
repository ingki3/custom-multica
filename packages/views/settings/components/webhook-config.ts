import type {
  IssueStatus,
  TaskFailureReason,
  WebhookEvent,
  WebhookFilters,
} from "@multica/core/types";

export const WEBHOOK_EVENT_OPTIONS = [
  {
    value: "issue.status_changed",
    label: "Issue status changed",
    description: "An issue moves from one workflow status to another.",
  },
  {
    value: "task.failed",
    label: "Task failed",
    description: "An agent task ends unsuccessfully.",
  },
  {
    value: "runtime.offline",
    label: "Runtime offline",
    description: "An agent runtime becomes unavailable.",
  },
  {
    value: "runtime.recovered",
    label: "Runtime recovered",
    description: "An unavailable runtime reconnects.",
  },
  {
    value: "server.ready",
    label: "Server ready",
    description: "The Multica server is ready to accept work.",
  },
] as const satisfies ReadonlyArray<{
  value: WebhookEvent;
  label: string;
  description: string;
}>;

export const WEBHOOK_STATUS_VALUES = [
  "backlog", "todo", "in_progress", "in_review", "done", "blocked", "cancelled",
] as const satisfies readonly IssueStatus[];

export const WEBHOOK_SOURCE_VALUES = [
  "task_started", "task_completed", "task_failed", "dependency_activation",
  "manual_update", "agent_cli", "batch_update",
] as const;

export const WEBHOOK_FAILURE_REASON_VALUES = [
  "agent_error", "auth_expired", "rate_limit", "quota_exceeded", "context_limit",
  "model_limit", "model_access", "model_not_found", "timeout", "runtime_offline",
  "runtime_recovery", "manual",
] as const satisfies readonly TaskFailureReason[];

export function parseWebhookFilterCsv<T extends string>(value: string, allowed: readonly T[]): T[] {
  const allowedValues = new Set(allowed);
  return [...new Set<T>(
    value.split(",").map((item) => item.trim()).filter(
      (item): item is T => item.length > 0 && allowedValues.has(item as T),
    ),
  )];
}

export function invalidWebhookFilterValues(value: string, allowed: readonly string[]): string[] {
  const allowedValues = new Set(allowed);
  return [...new Set(
    value.split(",").map((item) => item.trim()).filter(
      (item) => item.length > 0 && !allowedValues.has(item),
    ),
  )];
}

function nonEmpty<T extends string>(values: T[] | undefined): T[] | undefined {
  if (!values?.length) return undefined;
  const unique = [...new Set(values)];
  return unique.length ? unique : undefined;
}

export function normalizeWebhookConfig(
  events: readonly WebhookEvent[],
  filters: WebhookFilters,
): { events: WebhookEvent[]; filters: WebhookFilters } {
  const normalizedEvents = [...new Set(events)];
  if (normalizedEvents.length === 0) throw new Error("Select at least one event");

  const normalizedFilters: WebhookFilters = {};
  if (normalizedEvents.includes("issue.status_changed")) {
    const statusFrom = nonEmpty(filters.status_from);
    const statusTo = nonEmpty(filters.status_to);
    const source = nonEmpty(filters.source);
    if (statusFrom) normalizedFilters.status_from = statusFrom;
    if (statusTo) normalizedFilters.status_to = statusTo;
    if (source) normalizedFilters.source = source;
  }
  if (normalizedEvents.includes("task.failed")) {
    const failureReason = nonEmpty(filters.failure_reason);
    if (failureReason) normalizedFilters.failure_reason = failureReason;
    const willRetry = nonEmpty(filters.will_retry)?.filter(
      (value): value is "true" | "false" => value === "true" || value === "false",
    );
    if (willRetry?.length) {
      normalizedFilters.will_retry = willRetry;
    }
  }

  return { events: normalizedEvents, filters: normalizedFilters };
}