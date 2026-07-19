import type { AgentTask, IssueStatus } from "@multica/core/types";

const ACTIVE_TASK_STATUSES = new Set<AgentTask["status"]>([
  "queued",
  "dispatched",
  "running",
]);

export function shouldShowIssueTaskStatus(status: IssueStatus): boolean {
  return status === "in_progress" || status === "in_review";
}

export function findActiveIssueTask(
  issueId: string,
  issueStatus: IssueStatus,
  tasks: AgentTask[],
): AgentTask | null {
  if (!shouldShowIssueTaskStatus(issueStatus)) return null;

  return (
    tasks.find(
      (task) =>
        task.issue_id === issueId && ACTIVE_TASK_STATUSES.has(task.status),
    ) ?? null
  );
}
