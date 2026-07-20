package handler

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/service"
)

func TestTestWebhookPayloadIncludesGenericEventType(t *testing.T) {
	payload := testWebhookPayload("workspace-1", uuid.NewString(), service.EventIssueStatusChanged)
	if got := payload["event"]; got != service.EventIssueStatusChanged {
		t.Fatalf("event = %v, want %q", got, service.EventIssueStatusChanged)
	}
	if got := payload["event_type"]; got != service.EventIssueStatusChanged {
		t.Fatalf("event_type = %v, want %q", got, service.EventIssueStatusChanged)
	}
}

func TestNormalizeWebhookRequestAcceptsSupportedWebhookEvents(t *testing.T) {
	events := []string{
		service.EventIssueStatusChanged,
		service.EventTaskFailed,
		service.EventRuntimeOffline,
		service.EventRuntimeRecovered,
		service.EventServerReady,
	}
	raw, _, _, err := normalizeWebhookRequest(webhookRequest{Events: events})
	if err != nil {
		t.Fatalf("supported events rejected: %v", err)
	}
	var got []string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(events) {
		t.Fatalf("got %d events, want %d", len(got), len(events))
	}
}

func TestNormalizeWebhookRequestRejectsUnknownEvent(t *testing.T) {
	if _, _, _, err := normalizeWebhookRequest(webhookRequest{Events: []string{"unknown.event"}}); err == nil {
		t.Fatal("expected unknown event to be rejected")
	}
}

func TestNormalizeWebhookUpdatePreservesOmittedEventsAndFilters(t *testing.T) {
	events, filters, err := normalizeWebhookUpdateRequest(webhookRequest{Name: "renamed"})
	if err != nil {
		t.Fatal(err)
	}
	if events != nil || filters != nil {
		t.Fatalf("omitted update fields must remain nil, got events=%s filters=%s", events, filters)
	}
}

func TestNormalizeWebhookUpdateRejectsExplicitEmptyEvents(t *testing.T) {
	if _, _, err := normalizeWebhookUpdateRequest(webhookRequest{Events: []string{}}); err == nil {
		t.Fatal("expected explicit empty event selection to be rejected")
	}
}

func TestNormalizeWebhookUpdateSerializesSuppliedFields(t *testing.T) {
	events, filters, err := normalizeWebhookUpdateRequest(webhookRequest{
		Events:  []string{service.EventTaskFailed},
		Filters: map[string][]string{"will_retry": {"false"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(events) != `["task.failed"]` || string(filters) != `{"will_retry":["false"]}` {
		t.Fatalf("unexpected update payload: events=%s filters=%s", events, filters)
	}
}

func TestTestWebhookPayloadSupportsEveryOperationalEvent(t *testing.T) {
	for _, eventType := range service.SupportedWebhookEvents() {
		t.Run(eventType, func(t *testing.T) {
			payload := testWebhookPayload("workspace-1", uuid.NewString(), eventType)
			if err := service.ValidateWebhookEnvelope(payload); err != nil {
				t.Fatalf("invalid test envelope: %v", err)
			}
			switch eventType {
			case service.EventIssueStatusChanged:
				if payload["transition"] == nil {
					t.Fatal("missing transition")
				}
			case service.EventTaskFailed:
				task, ok := payload["task"].(map[string]any)
				if !ok || task["will_retry"] != false {
					t.Fatalf("invalid task failure sample: %#v", payload["task"])
				}
			case service.EventRuntimeOffline, service.EventRuntimeRecovered:
				if payload["runtime"] == nil {
					t.Fatal("missing runtime")
				}
			case service.EventServerReady:
				if payload["server"] == nil {
					t.Fatal("missing server")
				}
			}
		})
	}
}

func TestFirstWebhookEventUsesSubscriptionAndLegacyFallback(t *testing.T) {
	if got := firstWebhookEvent([]byte(`["task.failed","server.ready"]`)); got != service.EventTaskFailed {
		t.Fatalf("first event = %q", got)
	}
	for _, raw := range [][]byte{nil, []byte(`[]`), []byte(`not-json`)} {
		if got := firstWebhookEvent(raw); got != service.EventIssueStatusChanged {
			t.Fatalf("legacy fallback = %q", got)
		}
	}
}
