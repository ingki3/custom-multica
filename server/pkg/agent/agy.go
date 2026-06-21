package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
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

	args := buildAgyArgs(prompt, opts, b.cfg.Logger)
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

		startTime := time.Now()
		trySend(msgCh, Message{Type: MessageStatus, Status: "running"})
		out, err := cmd.Output()
		duration := time.Since(startTime)

		output := string(out)
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

		if output != "" {
			trySend(msgCh, Message{Type: MessageText, Content: output})
		}
		b.cfg.Logger.Info("agy finished", "status", finalStatus, "duration", duration.Round(time.Millisecond).String())

		resCh <- Result{
			Status:     finalStatus,
			Output:     output,
			Error:      finalError,
			DurationMs: duration.Milliseconds(),
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
