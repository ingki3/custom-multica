import { describe, expect, it } from "vitest";
import type {
  TaskFailureReason,
  WebhookEvent,
  WebhookFilters,
} from "@multica/core/types";
import {
  WEBHOOK_EVENT_OPTIONS,
  WEBHOOK_FAILURE_REASON_VALUES,
  invalidWebhookFilterValues,
  normalizeWebhookConfig,
  parseWebhookFilterCsv,
} from "./webhook-config";

describe("webhook config", () => {
  it("lists every supported webhook event", () => {
    expect(WEBHOOK_EVENT_OPTIONS.map(({ value }) => value)).toEqual([
      "issue.status_changed",
      "task.failed",
      "runtime.offline",
      "runtime.recovered",
      "server.ready",
    ] satisfies WebhookEvent[]);
  });

  it("includes model access and model-not-found failure reasons", () => {
    expect(WEBHOOK_FAILURE_REASON_VALUES).toEqual(expect.arrayContaining([
      "model_access",
      "model_not_found",
    ] satisfies TaskFailureReason[]));
  });

  it("requires at least one event", () => {
    expect(() => normalizeWebhookConfig([], {})).toThrow("Select at least one event");
  });

  it("preserves multiple selected events and removes duplicates", () => {
    expect(normalizeWebhookConfig(
      ["issue.status_changed", "task.failed", "task.failed"],
      {},
    ).events).toEqual(["issue.status_changed", "task.failed"]);
  });

  it("retains status and source filters only when the issue event is selected", () => {
    const filters = {
      status_from: ["todo"],
      status_to: ["done"],
      source: ["manual_update"],
      failure_reason: ["timeout"],
      will_retry: ["true"],
    } satisfies WebhookFilters;

    expect(normalizeWebhookConfig(["issue.status_changed"], filters).filters).toEqual({
      status_from: ["todo"],
      status_to: ["done"],
      source: ["manual_update"],
    });
    expect(normalizeWebhookConfig(["runtime.offline"], filters).filters).toEqual({});
  });

  it("retains failure filters only when task.failed is selected and validates will_retry", () => {
    expect(normalizeWebhookConfig(["task.failed"], {
      failure_reason: ["timeout"],
      will_retry: ["false"],
      status_to: ["done"],
    }).filters).toEqual({ failure_reason: ["timeout"], will_retry: ["false"] });

    expect(normalizeWebhookConfig(["task.failed"], {
      will_retry: ["sometimes"] as unknown as ("true" | "false")[],
    }).filters).toEqual({});
  });

  it("trims, validates, and de-duplicates comma-separated filter values", () => {
    expect(parseWebhookFilterCsv(" todo, nope,done, todo ", ["todo", "done"])).toEqual([
      "todo",
      "done",
    ]);
  });

  it("reports unsupported CSV values instead of silently dropping them", () => {
    expect(invalidWebhookFilterValues("todo, typo, done, typo", ["todo", "done"])).toEqual(["typo"]);
  });
});
