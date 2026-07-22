//go:build unix

package agent

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCursorExecuteSendsPromptOnStdinNotArgv(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	argvPath := filepath.Join(dir, "argv.txt")
	stdinPath := filepath.Join(dir, "stdin.txt")
	script := `#!/bin/sh
: > "$CURSOR_ARGV_PATH"
for a in "$@"; do printf '%s\n' "$a" >> "$CURSOR_ARGV_PATH"; done
cat > "$CURSOR_STDIN_PATH"
printf '%s\n' '{"type":"result","subtype":"success","is_error":false,"result":"ok"}'
`
	fakePath := filepath.Join(dir, "cursor-agent")
	writeTestExecutable(t, fakePath, []byte(script))
	backend, err := New("cursor", Config{
		ExecutablePath: fakePath,
		Logger:         slog.Default(),
		Env: map[string]string{
			"CURSOR_ARGV_PATH":  argvPath,
			"CURSOR_STDIN_PATH": stdinPath,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt := `go build -ldflags "-X main.version=foo" -o bin/server ./cmd/server`
	session, err := backend.Execute(t.Context(), prompt, ExecOptions{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range session.Messages {
		}
	}()
	result := <-session.Result
	argv, err := os.ReadFile(argvPath)
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != prompt {
		t.Fatalf("stdin = %q, want %q", stdin, prompt)
	}
	if strings.Contains(string(argv), "-X") || strings.Contains(string(argv), "ldflags") {
		t.Fatalf("prompt leaked into argv: %s", argv)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %s, error = %s", result.Status, result.Error)
	}
}

func TestCursorExecuteFailsOnCleanEOFWithoutResult(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fakePath := filepath.Join(dir, "cursor-agent")
	writeTestExecutable(t, fakePath, []byte("#!/bin/sh\ncat >/dev/null\nexit 0\n"))
	backend, err := New("cursor", Config{ExecutablePath: fakePath, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	session, err := backend.Execute(t.Context(), "prompt", ExecOptions{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range session.Messages {
		}
	}()
	result := <-session.Result
	if result.Status != "failed" || !strings.Contains(result.Error, "without a terminal result") {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Output != "" {
		t.Fatalf("failed result leaked output: %q", result.Output)
	}
}
