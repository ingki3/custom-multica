package main

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// TestResolveSelfHostServerURL covers GitHub #3912: `setup self-host` must
// honor MULTICA_SERVER_URL when --server-url is not passed, instead of always
// defaulting to localhost. The flag still wins over the env var.
func TestResolveSelfHostServerURL(t *testing.T) {
	newCmd := func() *cobra.Command {
		c := &cobra.Command{}
		c.Flags().String("server-url", "", "")
		c.Flags().Int("port", 8080, "")
		return c
	}

	t.Run("env var honored when flag absent", func(t *testing.T) {
		t.Setenv("MULTICA_SERVER_URL", "https://api.internal.co")
		serverURL, userProvided := resolveSelfHostServerURL(newCmd())
		if serverURL != "https://api.internal.co" {
			t.Fatalf("server_url: want env value, got %q", serverURL)
		}
		if !userProvided {
			t.Fatalf("userProvided: want true for env-sourced URL")
		}
	})

	t.Run("flag wins over env", func(t *testing.T) {
		t.Setenv("MULTICA_SERVER_URL", "https://env.example")
		cmd := newCmd()
		if err := cmd.Flags().Set("server-url", "https://flag.example"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
		serverURL, userProvided := resolveSelfHostServerURL(cmd)
		if serverURL != "https://flag.example" {
			t.Fatalf("server_url: want flag value, got %q", serverURL)
		}
		if !userProvided {
			t.Fatalf("userProvided: want true for flag-sourced URL")
		}
	})

	t.Run("falls back to localhost with --port when neither set", func(t *testing.T) {
		t.Setenv("MULTICA_SERVER_URL", "")
		cmd := newCmd()
		if err := cmd.Flags().Set("port", "9090"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
		serverURL, userProvided := resolveSelfHostServerURL(cmd)
		if serverURL != "http://localhost:9090" {
			t.Fatalf("server_url: want localhost default, got %q", serverURL)
		}
		if userProvided {
			t.Fatalf("userProvided: want false for localhost fallback")
		}
	})

	// MULTICA_SERVER_URL is documented as a ws:// daemon address; the probe and
	// stored config need an http(s) base, so the ws/wss + /ws form must be
	// normalized just like every other command does.
	t.Run("normalizes the documented ws:// daemon form", func(t *testing.T) {
		t.Setenv("MULTICA_SERVER_URL", "wss://api.internal.co/ws")
		serverURL, userProvided := resolveSelfHostServerURL(newCmd())
		if serverURL != "https://api.internal.co" {
			t.Fatalf("server_url: want normalized https base, got %q", serverURL)
		}
		if !userProvided {
			t.Fatalf("userProvided: want true for env-sourced URL")
		}
	})
}

// TestSelfHostAppURLHonorsEnv pins the app-url half of the GitHub #3912 fix:
// setup self-host resolves --app-url through the same FlagOrEnv path, so
// MULTICA_APP_URL is honored when the flag is absent.
func TestSelfHostAppURLHonorsEnv(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().String("app-url", "", "")
		return cmd
	}

	t.Run("env honored when flag absent", func(t *testing.T) {
		t.Setenv("MULTICA_APP_URL", "https://app.internal.co")
		if got := cli.FlagOrEnv(newCmd(), "app-url", "MULTICA_APP_URL", ""); got != "https://app.internal.co" {
			t.Fatalf("app_url: want env value, got %q", got)
		}
	})

	t.Run("flag wins over env", func(t *testing.T) {
		t.Setenv("MULTICA_APP_URL", "https://env.example")
		cmd := newCmd()
		if err := cmd.Flags().Set("app-url", "https://flag.example"); err != nil {
			t.Fatalf("set flag: %v", err)
		}
		if got := cli.FlagOrEnv(cmd, "app-url", "MULTICA_APP_URL", ""); got != "https://flag.example" {
			t.Fatalf("app_url: want flag value, got %q", got)
		}
	})
}

func TestServerHostIsLocal(t *testing.T) {
	cases := []struct {
		name   string
		server string
		want   bool
	}{
		{"localhost", "http://localhost:8080", true},
		{"127.0.0.1", "http://127.0.0.1:8080", true},
		{"IPv6 loopback", "http://[::1]:8080", true},
		{"LAN IP", "http://192.168.0.28:8080", false},
		{"public FQDN", "https://api.internal.co", false},
		{"unparseable", "://bad", false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := serverHostIsLocal(tc.server); got != tc.want {
				t.Errorf("serverHostIsLocal(%q) = %v, want %v", tc.server, got, tc.want)
			}
		})
	}
}
