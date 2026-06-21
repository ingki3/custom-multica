package agent

import (
	"log/slog"
	"strings"
	"testing"
)

func TestBuildAgyArgsBaseline(t *testing.T) {
	t.Parallel()

	args := buildAgyArgs("write a haiku", ExecOptions{}, slog.Default())
	expected := []string{"--print", "write a haiku"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d: %v", len(expected), len(args), args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Fatalf("expected args[%d] = %q, got %q", i, want, args[i])
		}
	}
}

func TestBuildAgyArgsWithModelAndResume(t *testing.T) {
	t.Parallel()

	args := buildAgyArgs("hi", ExecOptions{Model: "Gemini 3.5 Flash (Medium)", ResumeSessionID: "conv-1"}, slog.Default())
	joined := "\x00" + joinArgsForTest(args) + "\x00"
	if !containsArgPair(joined, "--model", "Gemini 3.5 Flash (Medium)") {
		t.Fatalf("expected --model flag, got %v", args)
	}
	if !containsArgPair(joined, "--conversation", "conv-1") {
		t.Fatalf("expected --conversation flag, got %v", args)
	}
}

func TestBuildAgyArgsFiltersBlockedCustomArgs(t *testing.T) {
	t.Parallel()

	args := buildAgyArgs("hi", ExecOptions{CustomArgs: []string{"--print", "--model", "bad", "--sandbox"}}, slog.Default())
	for i, a := range args {
		if a == "--model" && i+1 < len(args) && args[i+1] == "bad" {
			t.Fatalf("blocked --model bad should have been filtered: %v", args)
		}
	}
	if args[len(args)-2] != "--sandbox" {
		t.Fatalf("expected --sandbox to pass through before prompt, got %v", args)
	}
}

func joinArgsForTest(args []string) string {
	out := ""
	for i, arg := range args {
		if i > 0 {
			out += "\x00"
		}
		out += arg
	}
	return out
}

func containsArgPair(joined, flag, value string) bool {
	return strings.Contains(joined, "\x00"+flag+"\x00"+value+"\x00")
}
