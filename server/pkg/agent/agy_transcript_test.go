package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAgyTranscriptOutputReturnsOnlyLatestTurn(t *testing.T) {
	appData := t.TempDir()
	conversationID := "12345678-1234-1234-1234-123456789abc"
	logPath := filepath.Join(t.TempDir(), "agy.log")
	log := fmt.Sprintf("CLI app data directory: %s\nPrint mode: conversation=%s, sending message\n", appData, conversationID)
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(appData, "brain", conversationID, ".system_generated", "logs")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := "" +
		`{"type":"USER_INPUT","content":"old"}` + "\n" +
		`{"type":"PLANNER_RESPONSE","source":"MODEL","status":"DONE","content":"old reply"}` + "\n" +
		`{"type":"USER_INPUT","content":"new"}` + "\n" +
		`{"type":"PLANNER_RESPONSE","source":"MODEL","status":"RUNNING","content":"partial"}` + "\n" +
		`{"type":"PLANNER_RESPONSE","source":"MODEL","status":"DONE","content":"first"}` + "\n" +
		`{"type":"PLANNER_RESPONSE","source":"MODEL","status":"DONE","content":"second"}` + "\n"
	if err := os.WriteFile(filepath.Join(transcriptDir, "transcript.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := readAgyConversationID(logPath); got != conversationID {
		t.Fatalf("conversation ID = %q, want %q", got, conversationID)
	}
	if got := readAgyTranscriptOutput(logPath, conversationID, "new"); got != "first\n\nsecond" {
		t.Fatalf("recovered output = %q", got)
	}
}

func TestReadAgyTranscriptOutputDoesNotRecoverOldTurn(t *testing.T) {
	appData := t.TempDir()
	conversationID := "12345678-1234-1234-1234-123456789abc"
	logPath := filepath.Join(t.TempDir(), "agy.log")
	log := fmt.Sprintf("CLI app data directory: %s\nconversation=%s\n", appData, conversationID)
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(appData, "brain", conversationID, ".system_generated", "logs")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := `{"type":"PLANNER_RESPONSE","source":"MODEL","status":"DONE","content":"old reply"}` + "\n" +
		`{"type":"USER_INPUT","content":"new without reply"}` + "\n"
	if err := os.WriteFile(filepath.Join(transcriptDir, "transcript.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readAgyTranscriptOutput(logPath, conversationID, "new without reply"); got != "" {
		t.Fatalf("recovered stale output %q", got)
	}
}

func TestReadAgyTranscriptOutputRequiresCurrentInput(t *testing.T) {
	appData := t.TempDir()
	conversationID := "12345678-1234-1234-1234-123456789abc"
	logPath := filepath.Join(t.TempDir(), "agy.log")
	log := fmt.Sprintf("CLI app data directory: %s\nconversation=%s\n", appData, conversationID)
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}
	transcriptDir := filepath.Join(appData, "brain", conversationID, ".system_generated", "logs")
	if err := os.MkdirAll(transcriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := `{"type":"USER_INPUT","content":"previous prompt"}` + "\n" +
		`{"type":"PLANNER_RESPONSE","source":"MODEL","status":"DONE","content":"previous reply"}` + "\n"
	if err := os.WriteFile(filepath.Join(transcriptDir, "transcript.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readAgyTranscriptOutput(logPath, conversationID, "current prompt"); got != "" {
		t.Fatalf("recovered output without current input record: %q", got)
	}
}
