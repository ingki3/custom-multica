package main

import "testing"

func TestIssueDependencyCommandRegistered(t *testing.T) {
	depCmd, _, err := issueCmd.Find([]string{"dependency"})
	if err != nil {
		t.Fatalf("find dependency command: %v", err)
	}
	if depCmd == nil || depCmd.Name() != "dependency" {
		t.Fatalf("dependency command not registered: %#v", depCmd)
	}

	for _, sub := range []string{"list", "add", "requires", "then-runs", "remove"} {
		cmd, _, err := issueCmd.Find([]string{"dependency", sub})
		if err != nil {
			t.Fatalf("find dependency %s: %v", sub, err)
		}
		if cmd == nil || cmd.Name() != sub {
			t.Fatalf("dependency %s command not registered: %#v", sub, cmd)
		}
	}
}

func TestIssueDependencyAddRejectsInvalidDirection(t *testing.T) {
	cmd := issueDependencyAddCmd
	if err := cmd.Flags().Set("direction", "sideways"); err != nil {
		t.Fatalf("set direction: %v", err)
	}
	defer cmd.Flags().Set("direction", "prerequisite")

	err := runIssueDependencyAdd(cmd, []string{"BIZ-2", "BIZ-1"})
	if err == nil {
		t.Fatal("expected invalid direction error")
	}
	if got := err.Error(); got != "--direction must be prerequisite or next" {
		t.Fatalf("unexpected error: %s", got)
	}
}
