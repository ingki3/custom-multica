# Webhook Guide

Multica webhooks let external systems react to issue status changes.

Current event:

```text
issue.status_changed
```

This event fires when an issue status changes, with explicit `from` and `to`
values. The payload includes enough context to run follow-up automation through
the `multica` CLI or REST API.

## Create a Webhook

In the app:

1. Open **Settings**.
2. Open **Webhooks**.
3. Click **Add webhook**.
4. Enter:
   - Name
   - Target URL
   - Optional filters
5. Save the webhook.

The secret is shown when the webhook is created or rotated. Store it in the
receiving system; it is not shown again.

## Filters

Filters are optional. If no filter is set, the webhook receives every issue
status change.

Supported filters:

```text
From statuses
To statuses
Sources
```

Valid statuses:

```text
backlog
todo
in_progress
in_review
done
blocked
cancelled
```

Common sources:

```text
task_started
task_completed
task_failed
dependency_activation
manual_update
agent_cli
batch_update
```

Example: receive only agent completion review events:

```text
From statuses: in_progress
To statuses: in_review
Sources: task_completed
```

## Payload

Example payload:

```json
{
  "event": "issue.status_changed",
  "event_type": "issue.status_changed",
  "event_id": "4ce1e1a8-2f3b-4e27-a0ff-0b3d7db3bb18",
  "workspace": {
    "id": "8f4c2f8e-1d5d-4d19-9d73-4f2e2f2f8b11",
    "slug": "multica",
    "name": "Multica"
  },
  "issue": {
    "id": "9a5528d2-4b7a-4890-aac2-9d04e34f5a1a",
    "identifier": "MUL-123",
    "number": 123,
    "title": "Implement webhook delivery",
    "status": "in_review",
    "url": "http://localhost:3000/multica/issues/MUL-123",
    "api_url": "http://localhost:8080/api/issues/9a5528d2-4b7a-4890-aac2-9d04e34f5a1a"
  },
  "transition": {
    "from": "in_progress",
    "to": "in_review",
    "source": "task_completed",
    "occurred_at": "2026-06-03T12:34:56Z"
  },
  "actor": {
    "type": "system",
    "id": null
  },
  "task": {
    "id": "7b02f97b-d646-4f10-a431-58ec0da30a38",
    "agent_id": "3c77d71f-e9cc-4ef0-a7cb-695d5c70263f",
    "runtime_id": "2dd9d6f2-d44d-49a5-8b0d-d5c8e855a4d5",
    "completed_at": "2026-06-03T12:34:55Z"
  },
  "assignee": {
    "type": "agent",
    "id": "3c77d71f-e9cc-4ef0-a7cb-695d5c70263f",
    "name": "Review Agent"
  },
  "project": {
    "id": "5d3c3e8a-eac7-4e9a-aef3-56ed98c3c032",
    "title": "Webhook support"
  }
}
```

Important fields for automation:

```text
workspace.slug
issue.id
issue.identifier
transition.from
transition.to
transition.source
event_type
task.id
task.agent_id
project.id
```

Use `issue.identifier` for CLI commands and `issue.id` for REST API calls.

## Follow-Up Automation

Example shell handler:

```bash
#!/usr/bin/env bash
set -euo pipefail

payload="$(cat)"
issue_identifier="$(jq -r '.issue.identifier' <<< "$payload")"
from_status="$(jq -r '.transition.from' <<< "$payload")"
to_status="$(jq -r '.transition.to' <<< "$payload")"
source="$(jq -r '.transition.source' <<< "$payload")"

if [[ "$from_status" == "in_progress" && "$to_status" == "in_review" && "$source" == "task_completed" ]]; then
  multica issue comment "$issue_identifier" --content "External review workflow started."
fi
```

Other examples:

```bash
multica issue comment MUL-123 --content "QA started"
multica issue status MUL-123 in_progress
multica issue status MUL-123 done
```

The receiving system needs its own Multica authentication, usually a Personal
Access Token configured for the `multica` CLI or REST API client.

## Signature Verification

Every delivery includes these headers:

```text
X-Multica-Event: issue.status_changed
X-Multica-Delivery: <delivery_id>
X-Request-ID: <delivery_id>
X-Multica-Timestamp: <unix_seconds>
X-Multica-Signature: sha256=<hmac>
X-Webhook-Signature: <hmac_hex>
User-Agent: Multica-Webhooks/1.0
```

`X-Request-ID` mirrors `X-Multica-Delivery` for generic receivers that use
`X-Request-ID` as their idempotency or dedupe key.

`X-Multica-Signature` includes the timestamp in the signed input:

```text
<timestamp>.<raw_request_body>
```

Algorithm:

```text
HMAC-SHA256(secret, timestamp + "." + raw_body)
```

`X-Webhook-Signature` is provided for generic webhook adapters, including
Hermes Generic. It signs only the raw request body and is sent as raw hex
without a `sha256=` prefix:

```text
HMAC-SHA256(secret, raw_body)
```

## Hermes Generic Adapter

Multica is compatible with Hermes Generic webhook subscriptions:

```bash
hermes webhook subscribe multica-review \
  --description "Multica task_completed review handoff" \
  --events issue.status_changed \
  --prompt "Multica issue {issue.identifier} transitioned {transition.from} -> {transition.to}. Source={transition.source}. Task={task.id}. Assignee={assignee.name}. Payload: {__raw__}" \
  --deliver telegram
```

Hermes can identify the event through `payload.event_type`, verify
`X-Webhook-Signature` as raw HMAC-SHA256 hex, and dedupe retries through
`X-Request-ID`.

Recommended Multica filters for review handoff:

```text
From statuses: in_progress
To statuses: in_review
Sources: task_completed
```

Node.js verification example:

```js
import crypto from "node:crypto";

function verifyMulticaWebhook({ secret, timestamp, body, signature }) {
  const expected =
    "sha256=" +
    crypto
      .createHmac("sha256", secret)
      .update(`${timestamp}.${body}`)
      .digest("hex");

  return crypto.timingSafeEqual(Buffer.from(expected), Buffer.from(signature));
}
```

Reject requests with an old timestamp to reduce replay risk.

## Delivery and Retry

Multica sends webhook deliveries asynchronously. A slow or unavailable target
does not block issue status changes.

Delivery behavior:

```text
2xx response: delivered
network error or non-2xx response: retry
max attempts: 5
backoff: 1m, 5m, 15m, 1h, 6h
timeout: 10s
stored response body: first 4KB
```

In **Settings → Webhooks**, select a webhook to view recent deliveries. Failed
or retrying deliveries can be manually retried.

## Test Delivery

Use **Test** in the Webhooks settings tab to send a sample
`issue.status_changed` payload to the target URL.

Test deliveries are recorded in the delivery log just like real deliveries.

## API

Workspace-scoped API endpoints:

```text
GET    /api/webhooks
POST   /api/webhooks
GET    /api/webhooks/{id}
PUT    /api/webhooks/{id}
DELETE /api/webhooks/{id}
GET    /api/webhooks/{id}/deliveries
POST   /api/webhooks/{id}/test
POST   /api/webhooks/{id}/rotate-secret
POST   /api/webhook-deliveries/{id}/retry
```

Webhook management requires workspace `owner` or `admin` access.
