package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateFallbackCommentBody(t *testing.T) {
	t.Parallel()
	short := "I fixed the bug.\n\n- root cause\n- regression test"
	if got := truncateFallbackCommentBody(short, maxSynthesizedFallbackCommentRunes); got != short {
		t.Fatalf("short body changed: %q", got)
	}
	exact := strings.Repeat("你", maxSynthesizedFallbackCommentRunes)
	if got := truncateFallbackCommentBody(exact, maxSynthesizedFallbackCommentRunes); got != exact {
		t.Fatal("exact rune boundary was truncated")
	}
	oversized := strings.Repeat("tool call\n", maxSynthesizedFallbackCommentRunes)
	got := truncateFallbackCommentBody(oversized, maxSynthesizedFallbackCommentRunes)
	if got != oversizedFallbackCommentNotice {
		t.Fatalf("oversized body = %q, want safe notice", got)
	}
	if strings.Contains(got, "tool call") || !utf8.ValidString(got) {
		t.Fatalf("safe notice leaked raw output or invalid UTF-8: %q", got)
	}
}

func TestSanitizePersistedCommentContent(t *testing.T) {
	t.Parallel()
	got := sanitizePersistedCommentContent("before\x00after\xff")
	if got != "beforeafter\uFFFD" {
		t.Fatalf("sanitizePersistedCommentContent() = %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatal("sanitized comment must be valid UTF-8")
	}
}
