package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

const maxRemediationReasonRunes = 500

type issueRemediationResult struct {
	ID            string  `json:"id"`
	Action        string  `json:"action"`
	IssueID       string  `json:"issue_id"`
	CreatedTaskID *string `json:"created_task_id"`
	Idempotent    bool    `json:"idempotent"`
}

func newIssueRemediateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remediate <issue-id>",
		Short: "Atomically remediate a failed issue task",
		Long: "Atomically rerun, reassign and rerun, or block an issue after a final task failure.\n" +
			"Use a stable remediation key so replaying the same operation is idempotent.",
		Example: `  multica issue remediate BIZ-460 --failed-task <task-uuid> --action rerun \
    --key '<task-uuid>:timeout' --reason 'Automatic recovery after final timeout failure' --output json

  multica issue remediate BIZ-460 --failed-task <task-uuid> --action reassign-rerun \
    --target-agent 'Fallback Agent' --key '<task-uuid>:model_limit' \
    --reason 'Automatic fallback after final model_limit failure' --output json

  multica issue remediate BIZ-460 --failed-task <task-uuid> --action block \
    --key '<task-uuid>:auth_expired' --reason 'Operator action required: refresh provider authentication' --output json`,
		Args: exactArgs(1),
		RunE: runIssueRemediate,
	}
	cmd.Flags().String("failed-task", "", "UUID of the final failed task (required)")
	cmd.Flags().String("action", "", "Remediation action: rerun, reassign-rerun, or block (required)")
	cmd.Flags().String("target-agent", "", "Target agent UUID or name (required only for reassign-rerun)")
	cmd.Flags().String("key", "", "Stable idempotency key, typically <task-id>:<failure-reason> (required)")
	cmd.Flags().String("reason", "", fmt.Sprintf("Single-line operator-safe summary, at most %d characters (required)", maxRemediationReasonRunes))
	cmd.Flags().String("output", "table", "Output format: table or json")
	return cmd
}

func init() {
	issueCmd.AddCommand(newIssueRemediateCmd())
}

func runIssueRemediate(cmd *cobra.Command, args []string) error {
	failedTaskID, _ := cmd.Flags().GetString("failed-task")
	if strings.TrimSpace(failedTaskID) == "" {
		return fmt.Errorf("--failed-task is required")
	}
	if !uuidRegexp.MatchString(failedTaskID) {
		return fmt.Errorf("--failed-task must be a UUID")
	}

	action, _ := cmd.Flags().GetString("action")
	if action == "" {
		return fmt.Errorf("--action is required")
	}
	if action != "rerun" && action != "reassign-rerun" && action != "block" {
		return fmt.Errorf("invalid --action %q; valid values: rerun, reassign-rerun, block", action)
	}

	key, _ := cmd.Flags().GetString("key")
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("--key is required")
	}
	reason, _ := cmd.Flags().GetString("reason")
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("--reason is required")
	}
	if utf8.RuneCountInString(reason) > maxRemediationReasonRunes {
		return fmt.Errorf("--reason must be at most %d characters", maxRemediationReasonRunes)
	}
	for _, r := range reason {
		if unicode.IsControl(r) {
			return fmt.Errorf("--reason must be a single-line operator-safe summary without control characters")
		}
	}

	targetAgent, _ := cmd.Flags().GetString("target-agent")
	if action == "reassign-rerun" && strings.TrimSpace(targetAgent) == "" {
		return fmt.Errorf("--target-agent is required for action reassign-rerun")
	}
	if action != "reassign-rerun" && targetAgent != "" {
		return fmt.Errorf("--target-agent is only valid for action reassign-rerun")
	}
	output, _ := cmd.Flags().GetString("output")
	if output != "table" && output != "json" {
		return fmt.Errorf("invalid --output %q; valid values: table, json", output)
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	body := map[string]any{
		"failed_task_id":  failedTaskID,
		"action":          action,
		"remediation_key": key,
		"reason":          reason,
	}
	if action == "reassign-rerun" {
		targetAgentID, resolveErr := resolveAgent(ctx, client, strings.TrimSpace(targetAgent))
		if resolveErr != nil {
			return fmt.Errorf("resolve target agent: %w", resolveErr)
		}
		body["target_agent_id"] = targetAgentID
	}

	var result issueRemediationResult
	path := "/api/issues/" + args[0] + "/remediate-failure"
	if err := client.PostJSON(ctx, path, body, &result); err != nil {
		return fmt.Errorf("remediate issue failure: %w", err)
	}

	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	createdTaskID := "—"
	if result.CreatedTaskID != nil && *result.CreatedTaskID != "" {
		createdTaskID = *result.CreatedTaskID
	}
	cli.PrintTable(os.Stdout,
		[]string{"REMEDIATION", "ACTION", "ISSUE", "CREATED TASK", "IDEMPOTENT"},
		[][]string{{result.ID, result.Action, result.IssueID, createdTaskID, fmt.Sprintf("%t", result.Idempotent)}},
	)
	return nil
}
