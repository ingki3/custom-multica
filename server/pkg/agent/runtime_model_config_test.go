package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestListModelsReloadsExternalConfigOnEveryCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), runtimeModelsConfigName)
	t.Setenv(runtimeModelsConfigEnv, path)

	writeRuntimeModelsTestConfig(t, path, `{
  "version": 1,
  "providers": {
    "codex": {
      "models": [{"id":"first","label":"First","provider":"openai","default":true}]
    }
  }
}`)
	first, err := ListModels(context.Background(), "codex", "/ignored/codex")
	if err != nil {
		t.Fatalf("first ListModels: %v", err)
	}
	if len(first) != 1 || first[0].ID != "first" {
		t.Fatalf("first ListModels = %+v", first)
	}

	writeRuntimeModelsTestConfig(t, path, `{
  "version": 1,
  "providers": {
    "codex": {
      "models": [{"id":"second","label":"Second","provider":"openai","thinking":{"supported_levels":[{"value":"high","label":"High"}],"default_level":"high"}}]
    }
  }
}`)
	second, err := ListModels(context.Background(), "codex", "/ignored/codex")
	if err != nil {
		t.Fatalf("second ListModels: %v", err)
	}
	if len(second) != 1 || second[0].ID != "second" {
		t.Fatalf("second ListModels did not reload config: %+v", second)
	}
	if second[0].Thinking == nil || second[0].Thinking.DefaultLevel != "high" {
		t.Fatalf("thinking metadata was not loaded: %+v", second[0])
	}
}

func TestListModelsInitializesUserConfigWhenMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(runtimeModelsConfigEnv, "")

	models, err := ListModels(context.Background(), "claude", "")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected embedded defaults to initialize a non-empty Claude catalog")
	}
	path := filepath.Join(home, ".multica", runtimeModelsConfigName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read initialized config: %v", err)
	}
	if string(raw) != string(defaultRuntimeModelsConfig) {
		t.Fatal("initialized config differs from embedded default")
	}
}

func TestListModelsInitializesNamedProfileConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(runtimeModelsConfigEnv, "")
	t.Setenv(runtimeModelsProfileEnv, "work")

	if _, err := ListModels(context.Background(), "claude", ""); err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	path := filepath.Join(home, ".multica", "profiles", "work", runtimeModelsConfigName)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("named-profile config was not initialized: %v", err)
	}
}

func TestRuntimeModelsConfigConcurrentInitializationIsAtomic(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(runtimeModelsConfigEnv, "")
	t.Setenv(runtimeModelsProfileEnv, "")

	const callers = 20
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := configuredRuntimeModels("codex")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent configuredRuntimeModels: %v", err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(home, ".multica", runtimeModelsConfigName))
	if err != nil {
		t.Fatalf("read initialized config: %v", err)
	}
	if _, err := parseRuntimeModelsConfig(raw); err != nil {
		t.Fatalf("concurrently initialized config is invalid: %v", err)
	}
}

func TestListModelsRejectsInvalidProfileName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(runtimeModelsConfigEnv, "")
	t.Setenv(runtimeModelsProfileEnv, "../escape")

	_, err := ListModels(context.Background(), "claude", "")
	if err == nil || !strings.Contains(err.Error(), "invalid MULTICA_PROFILE") {
		t.Fatalf("expected invalid profile error, got %v", err)
	}
}

func TestListModelsExplicitMissingConfigFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	t.Setenv(runtimeModelsConfigEnv, path)

	_, err := ListModels(context.Background(), "claude", "")
	if err == nil || !strings.Contains(err.Error(), "read runtime models config") {
		t.Fatalf("expected explicit missing config error, got %v", err)
	}
}

func TestListModelsRejectsRelativeExplicitConfig(t *testing.T) {
	t.Setenv(runtimeModelsConfigEnv, "runtime-models.json")

	_, err := ListModels(context.Background(), "claude", "")
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("expected relative config path error, got %v", err)
	}
}

func TestListModelsRejectsOversizedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), runtimeModelsConfigName)
	if err := os.WriteFile(path, make([]byte, runtimeModelsConfigMaxSize+1), 0o644); err != nil {
		t.Fatalf("write oversized config: %v", err)
	}
	t.Setenv(runtimeModelsConfigEnv, path)

	_, err := ListModels(context.Background(), "claude", "")
	if err == nil || !strings.Contains(err.Error(), "file exceeds") {
		t.Fatalf("expected oversized config error, got %v", err)
	}
}

func TestParseRuntimeModelsConfigRejectsInvalidCatalogs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "unsupported version",
			raw:  `{"version":2,"providers":{"codex":{"models":[]}}}`,
			want: "unsupported version",
		},
		{
			name: "duplicate model",
			raw:  `{"version":1,"providers":{"codex":{"models":[{"id":"x","label":"X"},{"id":"x","label":"X again"}]}}}`,
			want: "duplicate model id",
		},
		{
			name: "multiple defaults",
			raw:  `{"version":1,"providers":{"codex":{"models":[{"id":"x","label":"X","default":true},{"id":"y","label":"Y","default":true}]}}}`,
			want: "at most one",
		},
		{
			name: "invalid default thinking level",
			raw:  `{"version":1,"providers":{"codex":{"models":[{"id":"x","label":"X","thinking":{"supported_levels":[{"value":"low","label":"Low"}],"default_level":"high"}}]}}}`,
			want: "default thinking level",
		},
		{
			name: "unknown field",
			raw:  `{"version":1,"providers":{"codex":{"models":[]}},"extra":true}`,
			want: "unknown field",
		},
		{
			name: "trailing JSON",
			raw:  `{"version":1,"providers":{"codex":{"models":[]}}} {}`,
			want: "multiple JSON values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRuntimeModelsConfig([]byte(tt.raw))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseRuntimeModelsConfig error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestDefaultRuntimeModelsConfigIsValid(t *testing.T) {
	cfg, err := parseRuntimeModelsConfig(defaultRuntimeModelsConfig)
	if err != nil {
		t.Fatalf("embedded runtime models config: %v", err)
	}
	for _, provider := range []string{
		"claude", "codex", "gemini", "cursor", "copilot", "hermes",
		"pi", "agy", "opencode", "openclaw", "kimi", "kiro",
	} {
		if _, ok := cfg.Providers[provider]; !ok {
			t.Errorf("embedded runtime models config missing provider %q", provider)
		}
	}
}

func writeRuntimeModelsTestConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write runtime models test config: %v", err)
	}
}
