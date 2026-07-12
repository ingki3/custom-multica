package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvPositiveIntOrDefault(t *testing.T) {
	const key = "MULTICA_TEST_ENV_INT"
	for _, tc := range []struct {
		value string
		want  int
	}{{"", 20}, {"0", 20}, {"-1", 20}, {"bad", 20}, {" 7 ", 7}} {
		t.Setenv(key, tc.value)
		if got := envPositiveIntOrDefault(key, 20); got != tc.want {
			t.Fatalf("value %q: got %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestNewDaemonLogRotatorDefaults(t *testing.T) {
	t.Setenv("MULTICA_DAEMON_LOG_MAX_SIZE_MB", "")
	t.Setenv("MULTICA_DAEMON_LOG_MAX_BACKUPS", "")
	t.Setenv("MULTICA_DAEMON_LOG_MAX_AGE_DAYS", "")
	path := filepath.Join(t.TempDir(), "daemon.log")
	rotator := newDaemonLogRotator(path)
	if rotator.Filename != path || rotator.MaxSize != 20 || rotator.MaxBackups != 5 || rotator.MaxAge != 30 || !rotator.Compress {
		t.Fatalf("unexpected rotator: %+v", rotator)
	}
}

func TestOpenBoundedErrLogRollsAtLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.err.log")
	if err := os.WriteFile(path+".1", []byte("stale backup"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, errLogMaxBytes), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := openBoundedErrLog(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("missing rolled backup: %v", err)
	}
	info, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(errLogMaxBytes) {
		t.Fatalf("rolled backup size = %d, want %d", info.Size(), errLogMaxBytes)
	}
}

func TestDaemonLogRotatorRotates(t *testing.T) {
	t.Setenv("MULTICA_DAEMON_LOG_MAX_SIZE_MB", "1")
	t.Setenv("MULTICA_DAEMON_LOG_MAX_BACKUPS", "2")
	dir := t.TempDir()
	path := filepath.Join(dir, "daemon.log")
	rotator := newDaemonLogRotator(path)
	rotator.Compress = false
	line := []byte(strings.Repeat("x", 1024) + "\n")
	for written := 0; written < 3*1024*1024; written += len(line) {
		if _, err := rotator.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := rotator.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("rotation produced only %d file(s): %v", len(entries), entries)
	}
}
