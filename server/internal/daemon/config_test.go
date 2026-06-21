package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDetectsAgyRuntime(t *testing.T) {
	dir := t.TempDir()
	agyPath := filepath.Join(dir, "agy")
	if err := os.WriteFile(agyPath, []byte("#!/bin/sh\necho 1.0.10\n"), 0o755); err != nil {
		t.Fatalf("write fake agy: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("MULTICA_AGY_MODEL", "Gemini 3.5 Flash (Medium)")

	cfg, err := LoadConfig(Overrides{})
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	entry, ok := cfg.Agents["agy"]
	if !ok {
		t.Fatalf("expected agy to be detected, agents=%v", cfg.Agents)
	}
	if entry.Path != "agy" {
		t.Fatalf("expected default agy path, got %q", entry.Path)
	}
	if entry.Model != "Gemini 3.5 Flash (Medium)" {
		t.Fatalf("expected agy model env, got %q", entry.Model)
	}
}
