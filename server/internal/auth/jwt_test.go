package auth

import (
	"strings"
	"testing"
)

func TestGenerateAgentTaskTokenPrefixAndLength(t *testing.T) {
	token, err := GenerateAgentTaskToken()
	if err != nil {
		t.Fatalf("GenerateAgentTaskToken() error = %v", err)
	}
	if !strings.HasPrefix(token, "mat_") {
		t.Fatalf("token prefix = %q, want mat_", token)
	}
	if got, want := len(token), len("mat_")+40; got != want {
		t.Fatalf("token length = %d, want %d", got, want)
	}
}

func TestGenerateAgentTaskTokenUnique(t *testing.T) {
	a, err := GenerateAgentTaskToken()
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	b, err := GenerateAgentTaskToken()
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if a == b {
		t.Fatal("expected unique tokens")
	}
}
