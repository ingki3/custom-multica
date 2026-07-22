# Multica CLI Manual for AI Agents

Last verified: 2026-07-19 against `go run ./cmd/multica --help` in `server/`.

This file is optimized for AI agents operating in a Multica workspace. It focuses on safe, repeatable command usage, JSON-friendly workflows, and common pitfalls. Prefer this file over guessing CLI syntax. When in doubt, run `multica <command> --help` or, from this repository, `cd server && go run ./cmd/multica <command> --help`.

## 0. Agent operating rules

- Do not print, log, commit, or expose tokens, API keys, passwords, or `.env` contents.
- Prefer `--output json` for commands whose output will be parsed by an agent.
- Use stable IDs from JSON output for follow-up commands. Human-readable issue identifiers may work for some issue routes, but UUIDs are safest.
- Before modifying an existing issue/project/agent, fetch it first with `get --output json`.
- For multiline issue/comment text, prefer `--description-stdin` or `--content-stdin` instead of shell-escaped strings.
- For file attachments, pass local file paths only. `http://` and `https://` URLs are not accepted by `--attachment`.
- Avoid retrying a failed `issue create` blindly after partial success. Check whether the issue was created first to avoid duplicates.
- Never paste secrets, raw provider errors, stack traces, prompts, or logs into `issue remediate --reason`. Use a bounded, single-line operator-safe summary.
- Use `--profile <name>` when operating a non-default local CLI profile. Profiles isolate config, daemon state, and workspace watches.
- Use `--workspace-id <uuid>` or `MULTICA_WORKSPACE_ID=<uuid>` when the active workspace is ambiguous.

## 1. Running the CLI from this repository

Installed binary:

```bash
multica --help
multica issue list --output json
```

Source checkout binary:

```bash
cd server
go run ./cmd/multica --help
go run ./cmd/multica issue list --output json
```

Repo helper:

```bash
make cli ARGS="issue list --output json"
```

Global flags:

```bash
multica --profile dev --server-url http://localhost:8080 --workspace-id <workspace-uuid> issue list --output json
```

Environment overrides:

```bash
MULTICA_SERVER_URL=http://localhost:8080
MULTICA_WORKSPACE_ID=<workspace-uuid>
MULTICA_RUNTIME_MODELS_CONFIG=~/.multica/runtime-models.json
```

`MULTICA_TOKEN` overrides the saved profile token for ordinary interactive and
CI commands. During daemon-managed agent execution, the CLI accepts only the
task-scoped `mat_` token injected by the daemon and never falls back to the
user's saved `mul_` PAT. A missing or non-task token fails closed.

## 2. Authentication and setup

Check config and auth:

```bash
multica config show
multica auth status
```

Configure server URLs:

```bash
multica config set server_url http://localhost:8080
multica config set app_url http://localhost:3000
multica config set workspace_id <workspace-uuid>
```

Login with browser:

```bash
multica login
```

Login with a Personal Access Token, without putting the token in the command line if possible:

```bash
multica login --token
```

Self-host setup:

```bash
multica setup self-host
```

Cloud setup:

```bash
multica setup cloud
```

Logout:

```bash
multica auth logout
```

## 3. Workspace discovery

List workspaces:

```bash
multica workspace list --output json
```

Get a workspace:

```bash
multica workspace get <workspace-slug-or-id> --output json
```

List members:

```bash
multica workspace members --output json
```

Recommended agent pattern:

```bash
multica workspace list --output json > /tmp/multica-workspaces.json
# Pick the intended workspace id, then use --workspace-id or MULTICA_WORKSPACE_ID.
```

## 3A. Developer-to-agent delegation workflow

Use this section when acting like a developer who creates work for a Multica agent, assigns it, checks the agent's result, comments back, and changes status.

### A. Preflight before assigning work

```bash
multica auth status
multica daemon status --output json
multica runtime list --output json
multica agent list --output json
```

Confirm:

- The CLI is authenticated to the intended server.
- The intended workspace is selected or passed with `--workspace-id`.
- The daemon is running if local agents should execute tasks.
- The target agent is visible, not archived, and has a runtime.

### B. Create an issue and immediately assign it to an agent

```bash
cat <<'EOF' | multica issue create \
  --title "Implement feature X" \
  --description-stdin \
  --status todo \
  --priority high \
  --assignee "Claude" \
  --output json
Goal:
- Implement feature X.

Context:
- Work in the repository's current branch.
- Follow CLAUDE.md / AGENTS.md.

Acceptance criteria:
- Tests pass.
- Summarize files changed and verification output in an issue comment.
EOF
```

Important execution semantics:

- Creating or assigning an issue to an agent can enqueue work when the status is not `backlog`.
- `backlog` is a parking lot. A backlog issue can be pre-assigned without starting work.
- Moving an agent-assigned issue from `backlog` to an active status such as `todo` triggers the agent if the runtime is ready.
- `done` and `cancelled` are terminal for normal work; `cancelled` cancels active tasks.

If you want to stage work without running it yet:

```bash
multica issue create --title "Staged task" --status backlog --assignee "Claude" --output json
# Later, start it:
multica issue status <issue-id> todo --output json
```

### C. Assign or reassign an existing issue

```bash
# Assign to an agent or member by display name.
multica issue assign <issue-id> --to "Claude" --output json

# If the issue is currently backlog, start it by moving out of backlog.
multica issue status <issue-id> todo --output json

# Reassign to another agent/member.
multica issue assign <issue-id> --to "Codex" --output json

# Remove assignee.
multica issue assign <issue-id> --unassign --output json
```

### D. Check whether work was queued, running, or completed

```bash
multica issue get <issue-id> --output json
multica issue runs <issue-id> --output json
```

Then inspect a run/task:

```bash
multica issue run-messages <task-id> --output json
multica issue run-messages <task-id> --since <sequence-number> --output json
```

Useful polling pattern for agents:

```bash
# 1. Fetch issue state.
multica issue get <issue-id> --output json

# 2. Fetch latest runs and identify the newest task id.
multica issue runs <issue-id> --output json

# 3. Fetch messages for the newest task.
multica issue run-messages <task-id> --output json
```

Fallback runs:

- A task may fail with a detailed `failure_reason` such as `auth_expired`, `rate_limit`, `quota_exceeded`, `context_limit`, or `model_limit`.
- If the source agent has a configured fallback agent and the reason is policy-allowed, Multica queues a child task for the fallback agent. The child task keeps `parent_task_id` pointing to the failed run.
- Authentication and quota failures do not fallback to another agent on the same provider; for example Codex `auth_expired` should be fixed with a real `codex login` refresh rather than retried as another Codex agent.
- When you are the fallback/sub agent, first inspect both the issue and parent run before editing:

```bash
multica issue get <issue-id> --output json
multica issue runs <issue-id> --output json
multica issue run-messages <parent-task-id> --output json
```

### E. Give feedback or request follow-up work

Add a comment. For agent-assigned issues, member comments can enqueue follow-up work when the agent/runtime is ready.

```bash
cat <<'EOF' | multica issue comment add <issue-id> --content-stdin --output json
Please address the failing typecheck and post the new verification output here.
EOF
```

Attach files when useful:

```bash
multica issue comment add <issue-id> \
  --content "Logs attached. Please investigate." \
  --attachment ./failure.log \
  --output json
```

### F. Mark status after reviewing results

```bash
# Move to review when the agent says implementation is ready.
multica issue status <issue-id> review --output json

# Mark done after the developer verifies the result.
multica issue status <issue-id> done --output json

# Cancel if the work is no longer needed or the task should stop.
multica issue status <issue-id> cancelled --output json
```

Recommended final verification loop:

```bash
multica issue get <issue-id> --output json
multica issue comment list <issue-id> --output json
multica issue runs <issue-id> --output json
```

## 4. Issues: read paths

List issues:

```bash
multica issue list --output json
multica issue list --status todo --limit 50 --output json
multica issue list --assignee "Claude" --output json
multica issue list --project <project-id> --output json
```

Get one issue:

```bash
multica issue get <issue-id-or-identifier> --output json
```

Search:

```bash
multica issue search "keyword" --output json
multica issue search "keyword" --include-closed --output json
```

Execution history and logs:

```bash
multica issue runs <issue-id> --output json
multica issue run-messages <task-id> --output json
multica issue run-messages <task-id> --since <sequence-number> --output json
```

Agent pattern for investigating a run:

```bash
issue_json=$(multica issue get <issue-id> --output json)
multica issue runs <issue-id> --output json
multica issue run-messages <task-id-from-runs> --output json
```

## 5. Issues: create and update

Create minimal issue:

```bash
multica issue create --title "Fix login redirect" --output json
```

Create with properties:

```bash
multica issue create \
  --title "Fix login redirect" \
  --description "Expected behavior..." \
  --status todo \
  --priority high \
  --assignee "Claude" \
  --project <project-id> \
  --due-date 2026-07-10T17:00:00Z \
  --output json
```

Create with multiline description:

```bash
cat <<'EOF' | multica issue create --title "Investigate websocket disconnect" --description-stdin --output json
Observed:
- Daemon logs show websocket close 1006.
- Backend health check failed during the same window.

Requested result:
- Identify root cause.
- Propose fix.
EOF
```

Create with attachments:

```bash
multica issue create \
  --title "Review screenshot" \
  --description "Screenshot attached." \
  --attachment ./screenshot.png \
  --attachment ./trace.txt \
  --output json
```

Attachment behavior for create:

- `--attachment` accepts local file paths only.
- The CLI pre-validates local files before creating the issue.
- After issue creation, the CLI uploads each file and links it to the new issue.
- If upload fails after creation, the issue remains created and the CLI warns on stderr. Do not blindly retry the whole create command; inspect the created issue first.

Update fields:

```bash
multica issue update <issue-id> --title "New title" --output json
multica issue update <issue-id> --status in_progress --output json
multica issue update <issue-id> --priority high --output json
multica issue update <issue-id> --assignee "Codex" --output json
multica issue update <issue-id> --project <project-id> --output json
multica issue update <issue-id> --parent <parent-issue-id> --output json
multica issue update <issue-id> --parent "" --output json
```

Update multiline description:

```bash
cat ./description.md | multica issue update <issue-id> --description-stdin --output json
```

Shortcut status/assignment commands:

```bash
multica issue status <issue-id> done --output json
multica issue assign <issue-id> --to "Claude" --output json
multica issue assign <issue-id> --unassign --output json
```

Rerun the current agent assignment:

```bash
multica issue rerun <issue-id> --output json
```

Atomically remediate a final failed task:

```bash
# Same-agent rerun.
multica issue remediate <issue-id> \
  --failed-task <failed-task-uuid> \
  --action rerun \
  --key <failed-task-uuid>:<failure-reason> \
  --reason "Automatic recovery after final timeout failure" \
  --output json

# Reassign and rerun. --target-agent accepts a UUID or an agent name.
multica issue remediate <issue-id> \
  --failed-task <failed-task-uuid> \
  --action reassign-rerun \
  --target-agent "Fallback Agent" \
  --key <failed-task-uuid>:<failure-reason> \
  --reason "Automatic fallback after final model_limit failure" \
  --output json

# Block without creating a task when operator action is required.
multica issue remediate <issue-id> \
  --failed-task <failed-task-uuid> \
  --action block \
  --key <failed-task-uuid>:<failure-reason> \
  --reason "Operator action required: refresh provider authentication" \
  --output json
```

Remediation safety and error handling:

- Use the final failed task's canonical UUID and a stable key, normally `<failed-task-id>:<failure-reason>`. Replaying the same key returns the existing remediation with `idempotent: true`; it does not create another task.
- `--target-agent` is required only for `reassign-rerun`. The server rejects the failed task's same agent as an unsafe fallback target and rejects archived, offline, or runtime-less targets.
- `--reason` is required, single-line, and limited to 500 Unicode characters. Summarize the operator decision; do not include raw errors or secrets.
- JSON output is stable and contains `id`, `action`, `issue_id`, `created_task_id` (null for block), and `idempotent`.
- Do not retry `active_task_exists` or `remediation_budget_exhausted`; re-fetch the issue and runs. A 404 means the failed task does not belong to the issue/workspace. A 422 indicates an invalid/non-failed task, unsafe reason/action, same-agent fallback, or an unavailable target agent. Correct operator input rather than retrying unchanged.
- This command is the only safe automatic recovery mutation. Do not implement reassign-and-rerun as separate `issue assign` and `issue rerun` calls because another task can start between those operations.

## 6. Issue dependencies

Issue dependencies control execution order.

Create with dependencies:

```bash
multica issue create \
  --title "Step B" \
  --requires <issue-A-id> \
  --then-runs <issue-C-id> \
  --output json
```

Add dependencies to an existing issue:

```bash
multica issue update <issue-id> --requires <prerequisite-issue-id> --output json
multica issue update <issue-id> --then-runs <next-issue-id> --output json
```

Dedicated dependency commands:

```bash
multica issue dependency list <issue-id> --output json
multica issue dependency requires <issue-id> <prerequisite-issue-id> --output json
multica issue dependency then-runs <issue-id> <next-issue-id> --output json
multica issue dependency add <issue-id> <target-issue-id> --direction prerequisite --output json
multica issue dependency add <issue-id> <target-issue-id> --direction next --output json
multica issue dependency remove <issue-id> <dependency-id> --output json
```

Semantics:

- `--requires <issue-id>`: this issue cannot run until the specified prerequisite is done.
- `--then-runs <issue-id>`: when this issue is done, the specified next issue can auto-start if unblocked.
- Relationships are bidirectional in the product model.
- Circular dependencies are rejected.

## 7. Comments and attachments

List comments:

```bash
multica issue comment list <issue-id> --output json
```

Add comment:

```bash
multica issue comment add <issue-id> --content "Status update." --output json
```

Add multiline comment:

```bash
cat <<'EOF' | multica issue comment add <issue-id> --content-stdin --output json
What I checked:
- backend logs
- daemon status

Conclusion:
- no action needed
EOF
```

Reply to a comment:

```bash
multica issue comment add <issue-id> --parent <comment-id> --content "Reply text" --output json
```

Add comment with attachments:

```bash
multica issue comment add <issue-id> \
  --content "Logs attached." \
  --attachment ./server.log \
  --attachment ./screenshot.png \
  --output json
```

Delete comment:

```bash
multica issue comment delete <comment-id>
```

Download an attachment:

```bash
multica attachment download <attachment-id> -o ./downloads
```

## 8. Projects

List projects:

```bash
multica project list --output json
```

Create a project:

```bash
multica project create \
  --title "Migration" \
  --description "Migration work" \
  --status active \
  --icon "🚚" \
  --working-folder /absolute/path/to/repo \
  --output json
```

Get/update/status/delete:

```bash
multica project get <project-id> --output json
multica project update <project-id> --title "New title" --output json
multica project status <project-id> completed --output json
multica project delete <project-id>
```

Agent note: `--working-folder` is important for agent execution because it tells agents where to work locally.

## 9. Agents

List agents:

```bash
multica agent list --output json
```

Get agent:

```bash
multica agent get <agent-id-or-name> --output json
```

Create agent:

```bash
multica agent create \
  --name "Claude" \
  --runtime-id <runtime-id> \
  --model claude-sonnet-4-6 \
  --visibility workspace \
  --max-concurrent-tasks 6 \
  --instructions "Follow repository instructions and update issue status." \
  --output json
```

Secret-safe custom environment handling:

```bash
# Prefer file or stdin for secrets. Do not pass real secrets through --custom-env on the command line.
multica agent create --name "Agent" --runtime-id <runtime-id> --custom-env-file ./agent-env.json --output json
cat ./agent-env.json | multica agent update <agent-id> --custom-env-stdin --output json
```

Update/archive/restore/tasks:

```bash
multica agent update <agent-id> --model <model-id> --output json
multica agent archive <agent-id>
multica agent restore <agent-id>
multica agent tasks <agent-id> --output json
```

Codex model discovery notes:

- Daemons running Codex CLI 0.122.0 or newer discover the installed bundled model and reasoning-effort catalog dynamically with `codex debug models --bundled`.
- Older, missing, offline, or malformed Codex installations fall back to Multica's verified static catalog, including GPT-5.6 Sol/Terra/Luna.
- The model-list API can include optional `thinking.supported_levels` and `thinking.default_level` metadata. This metadata is informational until the local fork adds persisted per-agent `thinking_level` configuration.

Skills assigned to agents:

```bash
multica agent skills <command> --help
```

## 10. Skills

List/get/create/update/delete:

```bash
multica skill list --output json
multica skill get <skill-id-or-name> --output json
multica skill create --help
multica skill update --help
multica skill delete <skill-id>
```

Import skills:

```bash
multica skill import <url> --output json
```

Skill files:

```bash
multica skill files --help
```

## 11. Daemon and runtimes

Status:

```bash
multica daemon status
```

Start in background:

```bash
multica daemon start
```

Start in foreground for debugging:

```bash
multica daemon start --foreground
```

Useful daemon start flags:

```bash
multica daemon start \
  --max-concurrent-tasks 4 \
  --poll-interval 3s \
  --heartbeat-interval 15s \
  --agent-timeout 20m
```

Restart/stop:

```bash
multica daemon restart
multica daemon stop
```

Logs:

```bash
multica daemon logs -n 100
multica daemon logs -f
```

Runtimes:

```bash
multica runtime list --output json
multica runtime usage --output json
multica runtime activity --output json
multica runtime ping <runtime-id> --output json
multica runtime update <runtime-id> --help
```

Runtime model catalogs are stored as JSON text and are read again for every model-list request from the agent settings UI. The daemon creates the default file on first use:

- Default profile: `~/.multica/runtime-models.json`
- Named profile: `~/.multica/profiles/<profile>/runtime-models.json`
- Override: `MULTICA_RUNTIME_MODELS_CONFIG=/absolute/path/runtime-models.json`

Each provider owns a `models` array. Model entries require `id` and `label`; `provider`, `default`, and `thinking` are optional. At most one model per provider may have `default: true`. Invalid JSON or metadata is returned to the UI as an explicit model-list failure. Because the file is uncached, an atomic edit is visible on the next request and does not require `multica daemon restart`.

```json
{
  "version": 1,
  "providers": {
    "codex": {
      "models": [
        {
          "id": "gpt-5.6-sol",
          "label": "GPT-5.6-Sol",
          "provider": "openai",
          "default": true,
          "thinking": {
            "supported_levels": [
              { "value": "low", "label": "Low" },
              { "value": "high", "label": "High" }
            ],
            "default_level": "low"
          }
        }
      ]
    }
  }
}
```

Agent troubleshooting pattern:

```bash
multica daemon status
multica daemon logs -n 200
multica runtime list --output json
multica agent list --output json
```

## 12. Labels, subscribers, repos, autopilots

Discover exact subcommands with help:

```bash
multica label --help
multica issue label --help
multica issue subscriber --help
multica repo --help
multica autopilot --help
```

Common docs summary:

- `multica label`: workspace-level issue labels.
- `multica issue label`: attach/detach/list labels on an issue.
- `multica issue subscriber`: subscribe/unsubscribe users or agents to issue notifications.
- `multica repo checkout <url>`: clone a repository locally for agents.
- `multica autopilot`: manage scheduled or triggered agent automations.

## 13. Output handling patterns for agents

Prefer JSON and parse with a real parser:

```bash
multica issue list --output json > /tmp/issues.json
python3 - <<'PY'
import json
issues = json.load(open('/tmp/issues.json'))
print(len(issues) if isinstance(issues, list) else issues.keys())
PY
```

Capture create output before using IDs:

```bash
created=$(multica issue create --title "Example" --output json)
issue_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])' <<<"$created")
multica issue get "$issue_id" --output json
```

Safe multiline create from a file:

```bash
multica issue create --title "Implement feature" --description-stdin --output json < ./issue-body.md
```

Use stderr warnings carefully:

- Some commands print useful warnings to stderr while still returning success.
- For attachment uploads, stderr may say a file failed after the issue was already created.
- Always inspect exit code and JSON output separately when automating.

## 14. Common statuses, priorities, and IDs

Typical issue statuses:

- `backlog`
- `todo`
- `in_progress`
- `review`
- `done`
- `cancelled`

Typical priorities:

- `none`
- `low`
- `medium`
- `high`
- `urgent`

ID guidance:

- Prefer UUIDs from JSON output for automation.
- Issue identifiers such as `MUL-123` are convenient for humans and accepted by some issue commands, but not every API boundary accepts them.
- Project, agent, runtime, attachment, comment, and dependency IDs should be treated as UUIDs unless help explicitly says otherwise.

## 15. Failure handling checklist

If a command fails:

1. Re-run the same command with `--help` only if syntax is uncertain.
2. Check auth: `multica auth status`.
3. Check config: `multica config show`.
4. Check workspace: `multica workspace list --output json`; set `--workspace-id` if needed.
5. Check server: for local self-host, `curl http://localhost:8080/health`.
6. Check daemon if agent execution is involved: `multica daemon status` and `multica daemon logs -n 200`.
7. For partial creates with attachments, search/get the issue before retrying.
8. Never include tokens or secret env values in reports.

## 16. Help command index

```bash
multica --help
multica auth --help
multica login --help
multica setup --help
multica workspace --help
multica issue --help
multica issue create --help
multica issue update --help
multica issue comment --help
multica issue dependency --help
multica attachment --help
multica project --help
multica agent --help
multica skill --help
multica daemon --help
multica runtime --help
multica config --help
multica version
```

## 17. Maintenance notes for future agents

When CLI behavior changes, update this file in the same change. At minimum:

1. Run relevant help commands from `server/` with `go run ./cmd/multica ... --help`.
2. Update examples and flags in this file.
3. Update public docs if affected, especially `apps/docs/content/docs/cli.mdx` and `apps/docs/content/docs/cli.zh.mdx` where appropriate.
4. If setup/install behavior changes, update `CLI_INSTALL.md` and self-hosting docs.
5. Verify examples against the current CLI, not memory.
