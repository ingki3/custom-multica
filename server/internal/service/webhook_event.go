package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	EventIssueStatusChanged = "issue.status_changed"
	EventTaskFailed         = "task.failed"
	EventRuntimeOffline     = "runtime.offline"
	EventRuntimeRecovered   = "runtime.recovered"
	EventServerReady        = "server.ready"
)

var supportedWebhookEvents = []string{
	EventIssueStatusChanged,
	EventTaskFailed,
	EventRuntimeOffline,
	EventRuntimeRecovered,
	EventServerReady,
}

// SupportedWebhookEvents returns a copy of the public webhook event vocabulary.
func SupportedWebhookEvents() []string {
	return append([]string(nil), supportedWebhookEvents...)
}

func IsSupportedWebhookEvent(eventType string) bool {
	for _, supported := range supportedWebhookEvents {
		if eventType == supported {
			return true
		}
	}
	return false
}

// ValidateWebhookEnvelope validates fields shared by every operational webhook.
func ValidateWebhookEnvelope(payload map[string]any) error {
	event, ok := payload["event"].(string)
	if !ok || !IsSupportedWebhookEvent(event) {
		return fmt.Errorf("unsupported or missing event")
	}
	eventType, ok := payload["event_type"].(string)
	if !ok || eventType != event {
		return fmt.Errorf("event and event_type must match")
	}
	eventID, ok := payload["event_id"].(string)
	if !ok || uuid.Validate(strings.TrimSpace(eventID)) != nil {
		return fmt.Errorf("event_id must be a UUID")
	}
	workspace, ok := payload["workspace"].(map[string]any)
	if !ok {
		return fmt.Errorf("workspace is required")
	}
	workspaceID, ok := workspace["id"].(string)
	if !ok || strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace.id is required")
	}
	if schemaVersion, ok := payload["schema_version"].(int); !ok || schemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}
	occurredAt, ok := payload["occurred_at"].(string)
	if !ok {
		return fmt.Errorf("occurred_at is required")
	}
	if _, err := time.Parse(time.RFC3339, occurredAt); err != nil {
		return fmt.Errorf("occurred_at must be RFC3339")
	}
	return nil
}
