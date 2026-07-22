package main

import (
	"net"
	"testing"

	"github.com/spf13/cobra"
)

// testCmd returns a minimal cobra.Command with the --profile persistent flag
// registered, matching the rootCmd setup used in production.
func testCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("profile", "", "")
	return cmd
}

func TestResolveAppURL(t *testing.T) {
	cmd := testCmd()

	t.Run("prefers MULTICA_APP_URL", func(t *testing.T) {
		t.Setenv("MULTICA_APP_URL", "http://localhost:14000")
		t.Setenv("FRONTEND_ORIGIN", "http://localhost:13000")

		if got := resolveAppURL(cmd); got != "http://localhost:14000" {
			t.Fatalf("resolveAppURL() = %q, want %q", got, "http://localhost:14000")
		}
	})

	t.Run("falls back to FRONTEND_ORIGIN", func(t *testing.T) {
		t.Setenv("MULTICA_APP_URL", "")
		t.Setenv("FRONTEND_ORIGIN", "http://localhost:13026")

		if got := resolveAppURL(cmd); got != "http://localhost:13026" {
			t.Fatalf("resolveAppURL() = %q, want %q", got, "http://localhost:13026")
		}
	})
}

func TestResolveTokenAgentContextIsFailClosed(t *testing.T) {
	cmd := testCmd()
	t.Setenv("MULTICA_AGENT_ID", "agent-1")
	t.Setenv("MULTICA_TASK_ID", "task-1")
	t.Setenv("MULTICA_TOKEN", "")
	if got := resolveToken(cmd); got != "" {
		t.Fatalf("agent context must not fall back to profile token, got %q", got)
	}
	t.Setenv("MULTICA_TOKEN", "mat_test_token")
	if got := resolveToken(cmd); got != "mat_test_token" {
		t.Fatalf("resolveToken() = %q", got)
	}
}

func TestNewAPIClientAgentContextRequiresTaskToken(t *testing.T) {
	cmd := testCmd()
	for key, value := range map[string]string{
		"MULTICA_AGENT_ID":     "agent-1",
		"MULTICA_TASK_ID":      "task-1",
		"MULTICA_SERVER_URL":   "http://127.0.0.1:8080",
		"MULTICA_WORKSPACE_ID": "workspace-1",
	} {
		t.Setenv(key, value)
	}
	t.Setenv("MULTICA_TOKEN", "mul_owner_token")
	if _, err := newAPIClient(cmd); err == nil {
		t.Fatal("expected owner token to be rejected in agent context")
	}
	t.Setenv("MULTICA_TOKEN", "mat_task_token")
	if _, err := newAPIClient(cmd); err != nil {
		t.Fatalf("task token rejected: %v", err)
	}
}

func TestResolveCallbackBinding(t *testing.T) {
	// Fake outbound detector: pretends the CLI has a fixed LAN IP regardless
	// of which server it dials.
	fixed := func(ip string) func(string) net.IP {
		return func(string) net.IP { return net.ParseIP(ip).To4() }
	}
	failing := func(string) net.IP { return nil }

	cases := []struct {
		name         string
		flagHost     string
		serverURL    string
		appURL       string
		detect       func(string) net.IP
		wantCallback string
		wantBind     string
	}{
		{
			name:         "public app URL stays on loopback",
			appURL:       "https://multica.ai",
			serverURL:    "https://api.multica.ai",
			detect:       failing,
			wantCallback: "localhost",
			wantBind:     "127.0.0.1",
		},
		{
			name:         "localhost app URL stays on loopback",
			appURL:       "http://localhost:3000",
			serverURL:    "http://localhost:8080",
			detect:       failing,
			wantCallback: "localhost",
			wantBind:     "127.0.0.1",
		},
		{
			name:         "same-machine self-host uses loopback (CLI IP matches app IP)",
			appURL:       "http://192.168.0.28:3000",
			serverURL:    "http://192.168.0.28:8080",
			detect:       fixed("192.168.0.28"),
			wantCallback: "localhost",
			wantBind:     "127.0.0.1",
		},
		{
			name:         "cross-machine self-host points callback at CLI's LAN IP",
			appURL:       "http://192.168.0.28:3000",
			serverURL:    "http://192.168.0.28:8080",
			detect:       fixed("192.168.0.47"),
			wantCallback: "192.168.0.47",
			wantBind:     "0.0.0.0",
		},
		{
			name:         "outbound detection failure falls back to app IP",
			appURL:       "http://192.168.0.28:3000",
			serverURL:    "http://192.168.0.28:8080",
			detect:       failing,
			wantCallback: "192.168.0.28",
			wantBind:     "0.0.0.0",
		},
		{
			name:         "--callback-host flag overrides everything",
			flagHost:     "cli.internal.example",
			appURL:       "https://multica.ai",
			serverURL:    "https://api.multica.ai",
			detect:       fixed("10.0.0.5"),
			wantCallback: "cli.internal.example",
			wantBind:     "0.0.0.0",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotCallback, gotBind := resolveCallbackBinding(tc.flagHost, tc.serverURL, tc.appURL, tc.detect)
			if gotCallback != tc.wantCallback {
				t.Errorf("callback host = %q, want %q", gotCallback, tc.wantCallback)
			}
			if gotBind != tc.wantBind {
				t.Errorf("bind addr = %q, want %q", gotBind, tc.wantBind)
			}
		})
	}
}

func TestNormalizeAPIBaseURL(t *testing.T) {
	t.Run("converts websocket base URL", func(t *testing.T) {
		if got := normalizeAPIBaseURL("ws://localhost:18106/ws"); got != "http://localhost:18106" {
			t.Fatalf("normalizeAPIBaseURL() = %q, want %q", got, "http://localhost:18106")
		}
	})

	t.Run("keeps http base URL", func(t *testing.T) {
		if got := normalizeAPIBaseURL("http://localhost:8080"); got != "http://localhost:8080" {
			t.Fatalf("normalizeAPIBaseURL() = %q, want %q", got, "http://localhost:8080")
		}
	})

	t.Run("falls back to raw value for invalid URL", func(t *testing.T) {
		if got := normalizeAPIBaseURL("://bad-url"); got != "://bad-url" {
			t.Fatalf("normalizeAPIBaseURL() = %q, want %q", got, "://bad-url")
		}
	})
}
