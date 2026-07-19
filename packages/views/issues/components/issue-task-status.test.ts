import { describe, expect, it } from "vitest";
import type { AgentTask } from "@multica/core/types";
import { findActiveIssueTask, shouldShowIssueTaskStatus } from "./issue-task-status";

function task(status: AgentTask["status"], issueId = "issue-1"): AgentTask {
  return {
    id: `task-${status}`,
    agent_id: "agent-1",
    runtime_id: "runtime-1",
    issue_id: issueId,
    status,
    priority: 0,
    dispatched_at: null,
    started_at: null,
    completed_at: null,
    result: null,
    error: null,
    created_at: "2026-07-19T00:00:00Z",
  };
}

describe("issue task status visibility", () => {
  it.each(["in_progress", "in_review"] as const)(
    "shows task status for %s issues",
    (status) => {
      expect(shouldShowIssueTaskStatus(status)).toBe(true);
    },
  );

  it.each(["backlog", "todo", "done", "blocked", "cancelled"] as const)(
    "hides task status for %s issues",
    (status) => {
      expect(shouldShowIssueTaskStatus(status)).toBe(false);
    },
  );

  it("finds queued and running tasks for an in-review issue", () => {
    expect(findActiveIssueTask("issue-1", "in_review", [task("queued")])?.status).toBe("queued");
    expect(findActiveIssueTask("issue-1", "in_review", [task("running")])?.status).toBe("running");
  });

  it("ignores terminal tasks and tasks for other issues", () => {
    expect(
      findActiveIssueTask("issue-1", "in_review", [
        task("completed"),
        task("running", "issue-2"),
      ]),
    ).toBeNull();
  });
});
