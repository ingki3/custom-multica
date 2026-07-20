import type { TaskFailureReason } from "./agent";
import type { IssueStatus } from "./issue";

export type WebhookEvent =
  | "issue.status_changed"
  | "task.failed"
  | "runtime.offline"
  | "runtime.recovered"
  | "server.ready";

export interface WebhookFilters {
  status_from?: IssueStatus[];
  status_to?: IssueStatus[];
  source?: string[];
  failure_reason?: TaskFailureReason[];
  will_retry?: ("true" | "false")[];
}

export interface WorkspaceWebhook {
  id: string;
  name: string;
  url: string;
  enabled: boolean;
  events: WebhookEvent[];
  filters: WebhookFilters;
  secret?: string;
  created_at: string;
  updated_at: string;
}

export interface WebhookDelivery {
  id: string;
  webhook_id: string;
  event_type: WebhookEvent;
  event_id: string;
  payload: Record<string, unknown>;
  status: "pending" | "retrying" | "delivered" | "failed";
  attempt_count: number;
  next_attempt_at: string;
  last_attempt_at: string | null;
  response_status: number | null;
  response_body: string | null;
  error: string | null;
  created_at: string;
  delivered_at: string | null;
}

export interface CreateWebhookRequest {
  name: string;
  url: string;
  enabled?: boolean;
  events?: WebhookEvent[];
  filters?: WebhookFilters;
}

export type UpdateWebhookRequest = Partial<CreateWebhookRequest>;
