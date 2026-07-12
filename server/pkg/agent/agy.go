package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// agyBackend implements Backend by spawning the Antigravity CLI (`agy`) in
// print mode. The CLI does not currently expose a structured streaming mode,
// so Multica reports the final stdout as one text event when the process exits.
type agyBackend struct {
	cfg Config
}

func (b *agyBackend) Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error) {
	execPath := b.cfg.ExecutablePath
	if execPath == "" {
		execPath = "agy"
	}
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("agy executable not found at %q: %w", execPath, err)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 20 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	logFile, err := os.CreateTemp("", "multica-agy-log-*.log")
	if err != nil {
		cancel()
		return nil, fmt.Errorf("create agy log file: %w", err)
	}
	logPath := logFile.Name()
	_ = logFile.Close()
	args := append(buildAgyArgs(prompt, opts, b.cfg.Logger), "--log-file", logPath)
	cmd := exec.CommandContext(runCtx, execPath, args...)
	hideAgentWindow(cmd)
	b.cfg.Logger.Info("agent command", "exec", execPath, "args", args)
	cmd.WaitDelay = 10 * time.Second
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	cmd.Env = buildEnv(b.cfg.Env)

	msgCh := make(chan Message, 2)
	resCh := make(chan Result, 1)

	go func() {
		defer cancel()
		defer close(msgCh)
		defer close(resCh)
		defer os.Remove(logPath)

		startTime := time.Now()
		trySend(msgCh, Message{Type: MessageStatus, Status: "running"})
		out, err := cmd.Output()
		duration := time.Since(startTime)

		output := string(out)
		sessionID := readAgyConversationID(logPath)
		finalStatus := "completed"
		var finalError string
		if runCtx.Err() == context.DeadlineExceeded {
			finalStatus = "timeout"
			finalError = fmt.Sprintf("agy timed out after %s", timeout)
		} else if runCtx.Err() == context.Canceled {
			finalStatus = "aborted"
			finalError = "execution cancelled"
		} else if err != nil {
			finalStatus = "failed"
			finalError = fmt.Sprintf("agy exited with error: %v", err)
			if ee, ok := err.(*exec.ExitError); ok {
				stderr := strings.TrimSpace(string(ee.Stderr))
				if stderr != "" {
					finalError = fmt.Sprintf("%s: %s", finalError, stderr)
				}
			}
		}
		if finalStatus == "completed" && strings.TrimSpace(output) == "" {
			if recovered := readAgyTranscriptOutput(logPath, sessionID, prompt); recovered != "" {
				output = recovered
				b.cfg.Logger.Info("agy recovered empty stdout from transcript", "bytes", len(recovered))
			}
		}

		if output != "" {
			trySend(msgCh, Message{Type: MessageText, Content: output})
		}
		b.cfg.Logger.Info("agy finished", "status", finalStatus, "duration", duration.Round(time.Millisecond).String())

		resCh <- Result{
			Status:     finalStatus,
			Output:     output,
			Error:      finalError,
			DurationMs: duration.Milliseconds(),
			SessionID:  sessionID,
			Usage:      map[string]TokenUsage{},
		}
	}()

	return &Session{Messages: msgCh, Result: resCh}, nil
}

var agyBlockedArgs = map[string]blockedArgMode{
	"-p":                             blockedStandalone,
	"--print":                        blockedStandalone,
	"--prompt":                       blockedStandalone,
	"--prompt-interactive":           blockedStandalone,
	"-i":                             blockedStandalone,
	"--dangerously-skip-permissions": blockedStandalone,
	"--model":                        blockedWithValue,
	"--conversation":                 blockedWithValue,
	"--log-file":                     blockedWithValue,
}

func buildAgyArgs(prompt string, opts ExecOptions, logger *slog.Logger) []string {
	if opts.SystemPrompt != "" {
		prompt = opts.SystemPrompt + "\n\n" + prompt
	}
	args := []string{"--print"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.ResumeSessionID != "" {
		args = append(args, "--conversation", opts.ResumeSessionID)
	}
	args = append(args, filterCustomArgs(opts.CustomArgs, agyBlockedArgs, logger)...)
	args = append(args, prompt)
	return args
}

var agyConversationIDRe = regexp.MustCompile(`conversation=([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)
var agyAppDataDirRe = regexp.MustCompile(`CLI app data directory:\s*(.+)`)

type agyTranscriptRecord struct {
	Type    string          `json:"type"`
	Source  string          `json:"source"`
	Status  string          `json:"status"`
	Content json.RawMessage `json:"content"`
}

func readAgyConversationID(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	matches := agyConversationIDRe.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return ""
	}
	return string(matches[len(matches)-1][1])
}

func readAgyAppDataDir(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	match := agyAppDataDirRe.FindSubmatch(data)
	if match == nil {
		return ""
	}
	return strings.TrimSpace(string(match[1]))
}

func readAgyTranscriptOutput(logPath, conversationID, currentPrompt string) string {
	if logPath == "" || conversationID == "" {
		return ""
	}
	appDataDir := readAgyAppDataDir(logPath)
	if appDataDir == "" {
		return ""
	}
	path := filepath.Join(appDataDir, "brain", conversationID, ".system_generated", "logs", "transcript.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	var parts []string
	foundCurrentInput := false
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		var record agyTranscriptRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if record.Type == "USER_INPUT" {
			parts = parts[:0]
			foundCurrentInput = strings.TrimSpace(agyTranscriptContentText(record.Content)) == strings.TrimSpace(currentPrompt)
			continue
		}
		if !foundCurrentInput || record.Type != "PLANNER_RESPONSE" || record.Source != "MODEL" || record.Status != "DONE" {
			continue
		}
		if text := agyTranscriptContentText(record.Content); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	if scanner.Err() != nil {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

func agyTranscriptContentText(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}
	var object struct {
		Text    string `json:"text"`
		Content string `json:"content"`
	}
	if json.Unmarshal(content, &object) != nil {
		return ""
	}
	if object.Text != "" {
		return object.Text
	}
	return object.Content
}
