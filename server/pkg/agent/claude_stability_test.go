package agent

import (
	"os"
	"strings"
	"testing"
)

func TestResolveSessionIDClearsStaleClaudeResumeFromStderr(t *testing.T) {
	got := resolveSessionID(
		"sess-dead",
		"sess-dead",
		true,
		"No conversation found with session ID: sess-dead",
	)
	if got != "" {
		t.Fatalf("resolveSessionID returned %q, want empty session for daemon fallback", got)
	}
}

func TestResolveSessionIDKeepsValidFailedResume(t *testing.T) {
	got := resolveSessionID("sess-valid", "sess-valid", true, "unrelated warning")
	if got != "sess-valid" {
		t.Fatalf("resolveSessionID returned %q, want existing valid session", got)
	}
}

func TestArgsRequestBypassPermissions(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"--permission-mode", "bypassPermissions"}, true},
		{[]string{"--dangerously-skip-permissions"}, true},
		{[]string{"--permission-mode", "default"}, false},
		{[]string{"--model", "sonnet"}, false},
	}
	for _, tc := range cases {
		if got := argsRequestBypassPermissions(tc.args); got != tc.want {
			t.Fatalf("argsRequestBypassPermissions(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestEnvHasSandboxUsesLastValue(t *testing.T) {
	for _, tc := range []struct {
		env  []string
		want bool
	}{
		{[]string{"IS_SANDBOX=1"}, true},
		{[]string{"IS_SANDBOX=TRUE"}, true},
		{[]string{"IS_SANDBOX=yes"}, true},
		{[]string{"IS_SANDBOX=on"}, true},
		{[]string{"IS_SANDBOX=1", "IS_SANDBOX=0"}, false},
		{[]string{"IS_SANDBOX=0", "IS_SANDBOX=1"}, true},
		{[]string{"PATH=/usr/bin"}, false},
	} {
		if got := envHasSandbox(tc.env); got != tc.want {
			t.Fatalf("envHasSandbox(%v) = %v, want %v", tc.env, got, tc.want)
		}
	}
}

func TestClaudeRootSudoPreflight(t *testing.T) {
	if err := claudeRootSudoPreflight([]string{"--permission-mode", "bypassPermissions"}, []string{"IS_SANDBOX=1"}); err != nil {
		t.Fatalf("sandbox bypass should be allowed: %v", err)
	}
	if err := claudeRootSudoPreflight([]string{"--permission-mode", "default"}, nil); err != nil {
		t.Fatalf("non-bypass should be allowed: %v", err)
	}
	if os.Geteuid() == 0 {
		err := claudeRootSudoPreflight([]string{"--permission-mode", "bypassPermissions"}, nil)
		if err == nil || !strings.Contains(err.Error(), "IS_SANDBOX") || !strings.Contains(err.Error(), "non-root") {
			t.Fatalf("expected actionable root guidance, got %v", err)
		}
	}
}
