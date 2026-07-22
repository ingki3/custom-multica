package daemon

import (
	"testing"

	"github.com/multica-ai/multica/server/pkg/agent"
)

func TestShouldRetryWithFreshSession(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		result   agent.Result
		prior    string
		tools    int32
		provider string
		want     bool
	}{
		{name: "explicit rejection", result: agent.Result{Status: "failed", ResumeRejected: true, SessionID: "stale"}, prior: "stale", provider: "claude", want: true},
		{name: "explicit rejection with network failure", result: agent.Result{Status: "failed", ResumeRejected: true, Error: "Connection closed mid-response"}, prior: "stale", provider: "claude"},
		{name: "tool side effect", result: agent.Result{Status: "failed", ResumeRejected: true}, prior: "stale", tools: 1, provider: "claude"},
		{name: "network", result: agent.Result{Status: "failed", Error: "Connection closed mid-response"}, prior: "stale", provider: "cursor"},
		{name: "rate limit", result: agent.Result{Status: "failed", Error: "429 rate limit exceeded"}, prior: "stale", provider: "cursor"},
		{name: "auth", result: agent.Result{Status: "failed", Error: "401 unauthorized auth token expired"}, prior: "stale", provider: "cursor"},
		{name: "compat startup failure", result: agent.Result{Status: "failed", Error: "exit status 1"}, prior: "stale", provider: "cursor", want: true},
		{name: "capable backend no signal", result: agent.Result{Status: "failed", Error: "exit status 1"}, prior: "stale", provider: "claude"},
		{name: "unknown backend fails closed", result: agent.Result{Status: "failed", Error: "exit status 1"}, prior: "stale", provider: "future"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldRetryWithFreshSession(tt.result, tt.prior, tt.tools, tt.provider); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClassifyAgentFailureProviderNetwork(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"API Error: Connection closed mid-response",
		"stream disconnected before completion",
	} {
		if got := classifyAgentFailure("claude", input); got != "provider_network" {
			t.Fatalf("classifyAgentFailure(%q) = %q", input, got)
		}
	}
	for _, input := range []string{
		"dial tcp: lookup database: no such host",
		"connection refused",
	} {
		if got := classifyAgentFailure("claude", input); got == "provider_network" {
			t.Fatalf("task-local failure %q must not be classified as provider_network", input)
		}
	}
}
