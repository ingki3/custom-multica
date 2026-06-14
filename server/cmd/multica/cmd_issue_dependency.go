package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

// multica issue dependency {list|add|requires|then-runs|remove} manages issue
// prerequisite/next relationships. The server stores one edge: next issue
// depends on prerequisite issue. The CLI exposes both user-facing directions.
var issueDependencyCmd = &cobra.Command{
	Use:     "dependency",
	Aliases: []string{"dependencies", "dep", "deps"},
	Short:   "Manage issue dependencies",
	Long: "Manage prerequisite and next-issue relationships.\n\n" +
		"Examples:\n" +
		"  multica issue dependency list BIZ-108\n" +
		"  multica issue dependency requires BIZ-108 BIZ-101\n" +
		"  multica issue dependency then-runs BIZ-108 BIZ-109\n" +
		"  multica issue dependency add BIZ-108 BIZ-101 --direction prerequisite\n" +
		"  multica issue dependency remove BIZ-108 <dependency-id>",
}

var issueDependencyListCmd = &cobra.Command{
	Use:   "list <issue-id>",
	Short: "List prerequisites and next issues",
	Args:  exactArgs(1),
	RunE:  runIssueDependencyList,
}

var issueDependencyAddCmd = &cobra.Command{
	Use:   "add <issue-id> <target-issue-id>",
	Short: "Add a dependency edge",
	Long: "Add a dependency edge. Use --direction prerequisite when the target must be done before the issue can run, " +
		"or --direction next when the target should auto-run after this issue is done.",
	Args: exactArgs(2),
	RunE: runIssueDependencyAdd,
}

var issueDependencyRequiresCmd = &cobra.Command{
	Use:   "requires <issue-id> <prerequisite-issue-id>",
	Short: "Add a prerequisite issue",
	Args:  exactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIssueDependencyAddWithDirection(cmd, args[0], args[1], "prerequisite")
	},
}

var issueDependencyThenRunsCmd = &cobra.Command{
	Use:   "then-runs <issue-id> <next-issue-id>",
	Short: "Add a next issue that auto-runs after this issue is done",
	Args:  exactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIssueDependencyAddWithDirection(cmd, args[0], args[1], "next")
	},
}

var issueDependencyRemoveCmd = &cobra.Command{
	Use:     "remove <issue-id> <dependency-id>",
	Aliases: []string{"delete", "rm"},
	Short:   "Remove a dependency by dependency id",
	Args:    exactArgs(2),
	RunE:    runIssueDependencyRemove,
}

type issueDependencyListResponse struct {
	Prerequisites []issueDependencyItem `json:"prerequisites"`
	NextIssues    []issueDependencyItem `json:"next_issues"`
}

type issueDependencyItem struct {
	ID         string `json:"id"`
	IssueID    string `json:"issue_id"`
	DependsOn  string `json:"depends_on_issue_id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	Identifier string `json:"identifier"`
}

func init() {
	issueDependencyCmd.AddCommand(issueDependencyListCmd)
	issueDependencyCmd.AddCommand(issueDependencyAddCmd)
	issueDependencyCmd.AddCommand(issueDependencyRequiresCmd)
	issueDependencyCmd.AddCommand(issueDependencyThenRunsCmd)
	issueDependencyCmd.AddCommand(issueDependencyRemoveCmd)

	issueDependencyListCmd.Flags().String("output", "table", "Output format: table or json")
	issueDependencyAddCmd.Flags().String("direction", "prerequisite", "Dependency direction: prerequisite or next")
	issueDependencyAddCmd.Flags().String("output", "table", "Output format: table or json")
	issueDependencyRequiresCmd.Flags().String("output", "table", "Output format: table or json")
	issueDependencyThenRunsCmd.Flags().String("output", "table", "Output format: table or json")
	issueDependencyRemoveCmd.Flags().String("output", "table", "Output format: table or json")

	issueCmd.AddCommand(issueDependencyCmd)
}

func runIssueDependencyList(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	deps, err := fetchIssueDependencies(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("list issue dependencies: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, deps)
	}
	printIssueDependencyTable(deps)
	return nil
}

func runIssueDependencyAdd(cmd *cobra.Command, args []string) error {
	direction, _ := cmd.Flags().GetString("direction")
	return runIssueDependencyAddWithDirection(cmd, args[0], args[1], direction)
}

func runIssueDependencyAddWithDirection(cmd *cobra.Command, issueID, targetIssueID, direction string) error {
	direction = strings.TrimSpace(strings.ToLower(direction))
	if direction != "prerequisite" && direction != "next" {
		return fmt.Errorf("--direction must be prerequisite or next")
	}

	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	body := map[string]any{"target_issue_id": targetIssueID, "direction": direction}
	var dep map[string]any
	if err := client.PostJSON(ctx, "/api/issues/"+issueID+"/dependencies", body, &dep); err != nil {
		return fmt.Errorf("add issue dependency: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, dep)
	}

	label := "prerequisite"
	if direction == "next" {
		label = "next issue"
	}
	fmt.Fprintf(os.Stdout, "Added %s %s to %s.\n", label, targetIssueID, issueID)
	if id := strVal(dep, "id"); id != "" {
		fmt.Fprintf(os.Stdout, "Dependency ID: %s\n", id)
	}
	return nil
}

func runIssueDependencyRemove(cmd *cobra.Command, args []string) error {
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	issueID, depID := args[0], args[1]
	if err := client.DeleteJSON(ctx, "/api/issues/"+issueID+"/dependencies/"+depID); err != nil {
		return fmt.Errorf("remove issue dependency: %w", err)
	}

	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, map[string]any{"dependency_deleted": true, "dependency_id": depID})
	}
	fmt.Fprintf(os.Stdout, "Dependency %s removed from %s.\n", depID, issueID)
	return nil
}

func fetchIssueDependencies(ctx context.Context, client *cli.APIClient, issueID string) (issueDependencyListResponse, error) {
	var deps issueDependencyListResponse
	if err := client.GetJSON(ctx, "/api/issues/"+issueID+"/dependencies", &deps); err != nil {
		return deps, err
	}
	return deps, nil
}

func printIssueDependencyTable(deps issueDependencyListResponse) {
	headers := []string{"DIRECTION", "IDENTIFIER", "STATUS", "TITLE", "DEPENDENCY ID"}
	rows := make([][]string, 0, len(deps.Prerequisites)+len(deps.NextIssues))
	for _, dep := range deps.Prerequisites {
		rows = append(rows, []string{"requires", dep.Identifier, dep.Status, dep.Title, truncateID(dep.ID)})
	}
	for _, dep := range deps.NextIssues {
		rows = append(rows, []string{"then-runs", dep.Identifier, dep.Status, dep.Title, truncateID(dep.ID)})
	}
	cli.PrintTable(os.Stdout, headers, rows)
}
