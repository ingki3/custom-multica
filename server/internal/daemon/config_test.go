package daemon

import (
	"os"
	"path/filepath"
	"strings"
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
	wantAgyPath := canonicalExecutablePath(agyPath)
	if entry.Path != wantAgyPath {
		t.Fatalf("expected canonical agy path %q, got %q", wantAgyPath, entry.Path)
	}
	if entry.Model != "Gemini 3.5 Flash (Medium)" {
		t.Fatalf("expected agy model env, got %q", entry.Model)
	}
}

func TestLoadConfigSkipsMulticaHooksWrapper(t *testing.T) {
	home := t.TempDir()
	hooksDir := filepath.Join(home, ".multica", "hooks")
	realBinDir := t.TempDir()
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(hooksDir, "codex"), filepath.Join(realBinDir, "codex")} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", hooksDir+string(os.PathListSeparator)+realBinDir)
	t.Setenv("MULTICA_DAEMON_ID", "11111111-1111-1111-1111-111111111111")

	cfg, err := LoadConfig(Overrides{WorkspacesRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Agents["codex"].Path; got != canonicalExecutablePath(filepath.Join(realBinDir, "codex")) {
		t.Fatalf("codex path = %q, want real binary outside hooks", got)
	}
}

func TestLoadConfigDoesNotRegisterHooksOnlyWrapper(t *testing.T) {
	home := t.TempDir()
	hooksDir := filepath.Join(home, ".multica", "hooks")
	realBinDir := t.TempDir()
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(hooksDir, "codex"), filepath.Join(realBinDir, "agy")} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := codexDesktopAppBundlePaths
	codexDesktopAppBundlePaths = func() []string { return nil }
	t.Cleanup(func() { codexDesktopAppBundlePaths = old })
	t.Setenv("HOME", home)
	t.Setenv("PATH", hooksDir+string(os.PathListSeparator)+realBinDir)
	t.Setenv("MULTICA_CODEX_PATH", "")
	t.Setenv("MULTICA_DAEMON_ID", "11111111-1111-1111-1111-111111111111")

	cfg, err := LoadConfig(Overrides{WorkspacesRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.Agents["codex"]; ok {
		t.Fatal("stale hooks-only codex wrapper was registered")
	}
	if _, ok := cfg.Agents["agy"]; !ok {
		t.Fatal("expected non-hook agy executable to remain discoverable")
	}
}

func TestLoadConfigUsesChatGPTAppBundleCodexPath(t *testing.T) {
	dir := t.TempDir()
	chatGPT := filepath.Join(dir, "ChatGPT.app", "Contents", "Resources", "codex")
	legacy := filepath.Join(dir, "Codex.app", "Contents", "Resources", "codex")
	for _, path := range []string{chatGPT, legacy} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	old := codexDesktopAppBundlePaths
	codexDesktopAppBundlePaths = func() []string { return []string{chatGPT, legacy} }
	t.Cleanup(func() { codexDesktopAppBundlePaths = old })
	t.Setenv("PATH", t.TempDir())
	t.Setenv("MULTICA_CODEX_PATH", "")
	t.Setenv("MULTICA_DAEMON_ID", "11111111-1111-1111-1111-111111111111")

	cfg, err := LoadConfig(Overrides{WorkspacesRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	wantChatGPT := canonicalExecutablePath(chatGPT)
	if got := cfg.Agents["codex"].Path; got != wantChatGPT {
		t.Fatalf("codex path = %q, want ChatGPT.app path %q", got, wantChatGPT)
	}
}

func TestCodexDesktopAppBundlePathsIncludesChatGPTAndLegacy(t *testing.T) {
	joined := strings.Join(codexDesktopAppBundlePaths(), "\n")
	for _, want := range []string{
		"ChatGPT.app/Contents/Resources/codex",
		"Codex.app/Contents/Resources/codex",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}
