package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const (
	testFailedTaskID  = "11111111-1111-4111-8111-111111111111"
	testTargetAgentID = "22222222-2222-4222-8222-222222222222"
)

func newRemediateTestCommand(serverURL string) *cobra.Command {
	cmd := newIssueRemediateCmd()
	cmd.Flags().String("server-url", "", "")
	cmd.Flags().String("workspace-id", "", "")
	cmd.Flags().String("profile", "", "")
	_ = cmd.Flags().Set("server-url", serverURL)
	_ = cmd.Flags().Set("workspace-id", "33333333-3333-4333-8333-333333333333")
	return cmd
}

func setRemediateFlags(t *testing.T, cmd *cobra.Command, action string) {
	t.Helper()
	for name, value := range map[string]string{
		"failed-task": testFailedTaskID,
		"action":      action,
		"key":         testFailedTaskID + ":timeout",
		"reason":      "Automatic recovery after final timeout failure",
		"output":      "json",
	} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	runErr := fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	_ = r.Close()
	return string(out), runErr
}

func TestIssueRemediateRequiredFlags(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*cobra.Command)
		want      string
	}{
		{"failed task", func(cmd *cobra.Command) {}, "--failed-task is required"},
		{"action", func(cmd *cobra.Command) { _ = cmd.Flags().Set("failed-task", testFailedTaskID) }, "--action is required"},
		{"key", func(cmd *cobra.Command) {
			_ = cmd.Flags().Set("failed-task", testFailedTaskID)
			_ = cmd.Flags().Set("action", "rerun")
		}, "--key is required"},
		{"reason", func(cmd *cobra.Command) {
			_ = cmd.Flags().Set("failed-task", testFailedTaskID)
			_ = cmd.Flags().Set("action", "rerun")
			_ = cmd.Flags().Set("key", "key")
		}, "--reason is required"},
		{"target agent for reassign", func(cmd *cobra.Command) {
			setRemediateFlags(t, cmd, "reassign-rerun")
		}, "--target-agent is required for action reassign-rerun"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRemediateTestCommand("http://unused.invalid")
			tt.configure(cmd)
			err := runIssueRemediate(cmd, []string{"BIZ-460"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestIssueRemediateValidatesInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cobra.Command)
		want   string
	}{
		{"failed task UUID", func(cmd *cobra.Command) { _ = cmd.Flags().Set("failed-task", "not-a-uuid") }, "--failed-task must be a UUID"},
		{"action", func(cmd *cobra.Command) { _ = cmd.Flags().Set("action", "retry") }, "invalid --action"},
		{"unexpected target", func(cmd *cobra.Command) { _ = cmd.Flags().Set("target-agent", testTargetAgentID) }, "--target-agent is only valid"},
		{"multiline reason", func(cmd *cobra.Command) { _ = cmd.Flags().Set("reason", "unsafe\nraw log") }, "single-line operator-safe summary"},
		{"bounded reason", func(cmd *cobra.Command) {
			_ = cmd.Flags().Set("reason", strings.Repeat("x", maxRemediationReasonRunes+1))
		}, "at most"},
		{"output", func(cmd *cobra.Command) { _ = cmd.Flags().Set("output", "yaml") }, "invalid --output"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRemediateTestCommand("http://unused.invalid")
			setRemediateFlags(t, cmd, "rerun")
			tt.mutate(cmd)
			err := runIssueRemediate(cmd, []string{"BIZ-460"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestIssueRemediateRequestShapeAndJSONOutput(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":              "remediation-1",
			"action":          "rerun",
			"issue_id":        "issue-1",
			"created_task_id": "task-2",
			"idempotent":      false,
			"internal":        "must not leak",
		})
	}))
	defer srv.Close()

	cmd := newRemediateTestCommand(srv.URL)
	setRemediateFlags(t, cmd, "rerun")
	out, err := captureStdout(t, func() error { return runIssueRemediate(cmd, []string{"BIZ-460"}) })
	if err != nil {
		t.Fatalf("runIssueRemediate: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/issues/BIZ-460/remediate-failure" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	wantBody := map[string]any{
		"failed_task_id":  testFailedTaskID,
		"action":          "rerun",
		"remediation_key": testFailedTaskID + ":timeout",
		"reason":          "Automatic recovery after final timeout failure",
	}
	if len(gotBody) != len(wantBody) {
		t.Fatalf("body = %#v, want %#v", gotBody, wantBody)
	}
	for key, want := range wantBody {
		if gotBody[key] != want {
			t.Errorf("body[%q] = %v, want %v", key, gotBody[key], want)
		}
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output %q: %v", out, err)
	}
	if len(result) != 5 || result["id"] != "remediation-1" || result["created_task_id"] != "task-2" || result["idempotent"] != false {
		t.Fatalf("JSON output = %#v", result)
	}
}

func TestIssueRemediateResolvesTargetAgentName(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/agents":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": testTargetAgentID, "name": "Fallback Agent"}})
		case "/api/issues/BIZ-460/remediate-failure":
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "remediation-1", "action": "reassign-rerun", "issue_id": "issue-1", "created_task_id": "task-2", "idempotent": false})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cmd := newRemediateTestCommand(srv.URL)
	setRemediateFlags(t, cmd, "reassign-rerun")
	_ = cmd.Flags().Set("target-agent", "Fallback Agent")
	_, err := captureStdout(t, func() error { return runIssueRemediate(cmd, []string{"BIZ-460"}) })
	if err != nil {
		t.Fatalf("runIssueRemediate: %v", err)
	}
	if gotBody["target_agent_id"] != testTargetAgentID {
		t.Fatalf("target_agent_id = %v, want %s", gotBody["target_agent_id"], testTargetAgentID)
	}
}

func TestIssueRemediateAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"unsafe_same_agent_target"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	cmd := newRemediateTestCommand(srv.URL)
	setRemediateFlags(t, cmd, "reassign-rerun")
	_ = cmd.Flags().Set("target-agent", testTargetAgentID)
	err := runIssueRemediate(cmd, []string{"BIZ-460"})
	if err == nil || !strings.Contains(err.Error(), "unsafe_same_agent_target") || !strings.Contains(err.Error(), "remediate issue failure") {
		t.Fatalf("error = %v", err)
	}
}
