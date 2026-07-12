package execenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCodexHomeCopiesRelativeModelCatalog(t *testing.T) {
	sharedHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(sharedHome, "config.toml"), []byte(`model_catalog_json = "cc-switch-model-catalog.json"`), 0o644); err != nil {
		t.Fatal(err)
	}
	want := `{"models":[{"model":"gpt-test"}]}`
	if err := os.WriteFile(filepath.Join(sharedHome, "cc-switch-model-catalog.json"), []byte(want), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", sharedHome)

	codexHome := filepath.Join(t.TempDir(), "codex-home")
	if err := prepareCodexHome(codexHome, testLogger()); err != nil {
		t.Fatalf("prepareCodexHome: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(codexHome, "cc-switch-model-catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("catalog = %q, want %q", data, want)
	}
}

func TestPrepareCodexHomeReportsMissingModelCatalogPath(t *testing.T) {
	sharedHome := t.TempDir()
	if err := os.WriteFile(filepath.Join(sharedHome, "config.toml"), []byte(`model_catalog_json = "missing-catalog.json"`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", sharedHome)

	err := prepareCodexHome(filepath.Join(t.TempDir(), "codex-home"), testLogger())
	if err == nil {
		t.Fatal("expected missing catalog error")
	}
	for _, want := range []string{"model_catalog_json", "missing-catalog.json", sharedHome} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}
