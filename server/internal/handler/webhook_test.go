package handler

import (
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
)

func TestTestWebhookPayloadIncludesGenericEventType(t *testing.T) {
	payload := testWebhookPayload("workspace-1", "event-1")

	if got := payload["event"]; got != service.EventIssueStatusChanged {
		t.Fatalf("event = %v, want %q", got, service.EventIssueStatusChanged)
	}
	if got := payload["event_type"]; got != service.EventIssueStatusChanged {
		t.Fatalf("event_type = %v, want %q", got, service.EventIssueStatusChanged)
	}
}
