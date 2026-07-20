package service

import "testing"

func TestSupportedWebhookEvents(t *testing.T) {
	for _, event := range []string{
		EventIssueStatusChanged,
		EventTaskFailed,
		EventRuntimeOffline,
		EventRuntimeRecovered,
		EventServerReady,
	} {
		if !IsSupportedWebhookEvent(event) {
			t.Fatalf("event %q should be supported", event)
		}
	}
	if IsSupportedWebhookEvent("unknown.event") {
		t.Fatal("unknown event should not be supported")
	}
}

func TestValidateWebhookEnvelope(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{
			"schema_version": 1,
			"event":          EventTaskFailed,
			"event_type":     EventTaskFailed,
			"event_id":       "8fda69a3-5325-49f4-9822-146466e71fc1",
			"occurred_at":    "2026-07-19T12:34:56Z",
			"workspace": map[string]any{
				"id": "a98f85b8-2055-4c7b-93ad-fdef53343c14",
			},
		}
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"event mismatch", func(p map[string]any) { p["event"] = EventServerReady }},
		{"invalid event id", func(p map[string]any) { p["event_id"] = "not-a-uuid" }},
		{"missing workspace id", func(p map[string]any) { p["workspace"] = map[string]any{} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := valid()
			tt.mutate(payload)
			if err := ValidateWebhookEnvelope(payload); err == nil {
				t.Fatal("expected invalid envelope")
			}
		})
	}
	if err := ValidateWebhookEnvelope(valid()); err != nil {
		t.Fatalf("valid envelope rejected: %v", err)
	}
}
