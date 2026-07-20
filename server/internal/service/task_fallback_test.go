package service

import "testing"

func TestFallbackReasonAllowed(t *testing.T) {
	t.Parallel()

	allowed := []string{"context_limit", "rate_limit", "model_limit", "model_access", "model_not_found"}
	if !fallbackReasonAllowed("context_limit", allowed) {
		t.Fatal("context_limit should be allowed")
	}
	if !fallbackReasonAllowed("model_access", allowed) {
		t.Fatal("model_access should be explicitly allowed")
	}
	if !fallbackReasonAllowed("model_not_found", allowed) {
		t.Fatal("model_not_found should be explicitly allowed")
	}
	if fallbackReasonAllowed("auth_expired", allowed) {
		t.Fatal("auth_expired should not be allowed by default")
	}
	if fallbackReasonAllowed("agent_error", allowed) {
		t.Fatal("agent_error should not be allowed by default")
	}
}

func TestFallbackProviderAllowedBlocksSameProviderAuthAndQuota(t *testing.T) {
	t.Parallel()

	if fallbackProviderAllowed("auth_expired", "codex", "codex") {
		t.Fatal("same-provider auth_expired fallback should be blocked")
	}
	if fallbackProviderAllowed("quota_exceeded", "codex", "codex") {
		t.Fatal("same-provider quota_exceeded fallback should be blocked")
	}
	if !fallbackProviderAllowed("auth_expired", "codex", "claude") {
		t.Fatal("cross-provider auth_expired fallback may be allowed by explicit policy")
	}
	if !fallbackProviderAllowed("context_limit", "codex", "codex") {
		t.Fatal("same-provider context_limit fallback should be allowed")
	}
}
