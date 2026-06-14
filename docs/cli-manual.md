# Multica CLI Manual

This manual documents the locally managed Multica CLI. Keep it updated whenever `server/cmd/multica/*` changes, especially when commands, flags, output formats, or setup behavior change.

Last verified against local CLI:

```bash
multica --version
# multica be887534b (commit: be887534b, built: 2026-06-14T08:53:08Z)
```

## Quick reference

```bash
multica setup self-host
multica auth status
multica workspace list
multica project list
multica issue list
multica issue create --title "Design review" --description "Check design gaps" --project <project-id>
multica issue update <issue-id> --status in_progress
multica issue dependency requires <issue-id> <prerequisite-issue-id>
multica issue dependency then-runs <issue-id> <next-issue-id>
multica daemon status
multica daemon logs -f
```

## Configuration and auth

The CLI stores user configuration under the user's Multica config directory, usually:

```text
/Users/simplist/.multica/config.json
```

Do not paste or commit tokens from this file. When documenting/debugging, redact secrets as `[REDACTED]`.

### `multica setup`

Configure the CLI, authenticate, and start the daemon.

```bash
multica setup cloud
multica setup self-host
```

Use `self-host` for the local/self-hosted dev stack. Recent setup behavior supports environment URLs such as `MULTICA_SERVER_URL` / `MULTICA_APP_URL` when flags are omitted.

### `multica auth`

```bash
multica auth status
multica auth logout
```

Use `auth status` to verify whether the current config token is valid.

## Workspaces

```bash
multica workspace list
multica workspace get <workspace-id-or-slug>
multica workspace members <workspace-id-or-slug>
```

Common use:

```bash
multica workspace list --output table
multica workspace members bizcatalyst
```

## Projects

```bash
multica project list
multica project get <project-id>
multica project create --name "Admin 2.0"
multica project update <project-id> --name "Admin 2.0" --description "..."
multica project status <project-id> <status>
multica project delete <project-id>
```

Project IDs can be used with issue commands via `--project <project-id>`.

## Issues

### List and inspect

```bash
multica issue list
multica issue search "admin"
multica issue get <issue-id>
multica issue runs <issue-id>
multica issue run-messages <run-id>
```

### Create

```bash
multica issue create --title "Implement settings page" \
  --description "Build the settings page" \
  --project <project-id> \
  --assignee "codex" \
  --priority high \
  --status todo
```

Important create flags:

```text
--title string             Issue title (required)
--description string       Issue description; decodes \n, \r, \t, \\
--description-stdin        Read description from stdin verbatim
--assignee string          Member or agent name
--attachment strings       File path(s) to attach; can repeat
--project string           Project ID
--parent string            Parent issue ID
--priority string          Issue priority
--status string            Issue status
--due-date string          RFC3339 due date
--requires strings         Prerequisite issue IDs
--then-runs strings        Next issue IDs to auto-run after this issue is done
--output string            table or json; default json
```

Multi-line description examples:

```bash
multica issue create --title "Spec" --description $'line 1\n\nline 2'

cat spec.md | multica issue create --title "Spec" --description-stdin
```

Create an ordered chain directly:

```bash
multica issue create --title "Build UI" --requires BIZ-101 --then-runs BIZ-103
```

### Update

```bash
multica issue update <issue-id> --title "New title"
multica issue update <issue-id> --status in_progress
multica issue update <issue-id> --assignee "codex"
multica issue update <issue-id> --parent ""   # clear parent
```

Important update flags mirror create:

```text
--title string
--description string
--description-stdin
--assignee string
--project string
--parent string
--priority string
--status string
--due-date string
--requires strings
--then-runs strings
--output string            table or json; default json
```

### Status and assignment

```bash
multica issue status <issue-id> <status>
multica issue assign <issue-id> <member-or-agent-name>
multica issue rerun <issue-id>
```

`rerun` re-enqueues the issue's current agent assignment as a fresh task.

## Issue dependencies

Issue dependencies control execution order.

Concepts:

- Prerequisite / `requires`: another issue must be `done` before this issue can run.
- Next issue / `then-runs`: another issue should automatically move forward after this issue is `done`, if all of its prerequisites are satisfied.
- The server stores one edge: `next issue depends on prerequisite issue`.

### List dependencies

```bash
multica issue dependency list <issue-id>
multica issue dependency list <issue-id> --output json
```

Aliases for `dependency`:

```bash
multica issue dependencies list <issue-id>
multica issue dep list <issue-id>
multica issue deps list <issue-id>
```

### Add a prerequisite

Use either the semantic command:

```bash
multica issue dependency requires <issue-id> <prerequisite-issue-id>
```

or the generic command:

```bash
multica issue dependency add <issue-id> <prerequisite-issue-id> --direction prerequisite
```

Example:

```bash
# BIZ-108 cannot run until BIZ-101 is done.
multica issue dependency requires BIZ-108 BIZ-101
```

### Add a next issue

Use either the semantic command:

```bash
multica issue dependency then-runs <issue-id> <next-issue-id>
```

or the generic command:

```bash
multica issue dependency add <issue-id> <next-issue-id> --direction next
```

Example:

```bash
# When BIZ-108 is done, BIZ-109 can auto-run if ready.
multica issue dependency then-runs BIZ-108 BIZ-109
```

### Remove a dependency

First list dependencies to find the dependency ID:

```bash
multica issue dependency list BIZ-108
```

Then remove it:

```bash
multica issue dependency remove BIZ-108 <dependency-id>
```

Aliases:

```bash
multica issue dependency delete BIZ-108 <dependency-id>
multica issue dependency rm BIZ-108 <dependency-id>
```

## Comments, labels, and subscribers

### Comments

```bash
multica issue comment list <issue-id>
multica issue comment add <issue-id> --body "Looks good"
multica issue comment delete <issue-id> <comment-id>
```

### Labels

```bash
multica issue label list <issue-id>
multica issue label add <issue-id> <label-id>
multica issue label remove <issue-id> <label-id>
```

Labels themselves are managed by the top-level label commands if present in the installed CLI.

### Subscribers

```bash
multica issue subscriber list <issue-id>
multica issue subscriber add <issue-id>
multica issue subscriber remove <issue-id>
```

Subscriber commands can also target another user/agent when supported by the command help.

## Agents

```bash
multica agent list
multica agent get <agent-id>
multica agent create --name "codex-worker"
multica agent update <agent-id> ...
multica agent archive <agent-id>
multica agent restore <agent-id>
multica agent tasks <agent-id>
multica agent skills <command>
```

Use `multica agent <command> --help` for exact flags because agent configuration evolves frequently.

## Daemon

The daemon is the local runtime that claims and executes agent tasks.

```bash
multica daemon start
multica daemon stop
multica daemon restart
multica daemon status
multica daemon logs
multica daemon logs -f
```

For local Multica development, run daemon commands with the user's real home so the daemon reads the durable CLI config:

```bash
HOME=/Users/simplist multica daemon status
```

## Runtimes

```bash
multica runtime list
multica runtime activity <runtime-id>
multica runtime usage <runtime-id>
multica runtime update <runtime-id>
```

## MCP

If the installed CLI exposes `mcp`, inspect the exact command tree with:

```bash
multica mcp --help
```

MCP support is evolving. Prefer the command help as the source of truth until this section is expanded with stable examples.

## Output formats

Most commands support:

```bash
--output table
--output json
```

Use `json` for scripts and `table` for human inspection.

Example:

```bash
multica issue dependency list BIZ-108 --output json
```

## Maintenance workflow for this manual

When changing the CLI:

1. Update the command implementation under `server/cmd/multica/`.
2. Add or update tests under `server/cmd/multica/*_test.go`.
3. Run targeted checks:
   ```bash
   cd server && go test ./cmd/multica
   ```
4. Rebuild/install locally if needed:
   ```bash
   make build
   install -m 0755 server/bin/multica /opt/homebrew/bin/multica
   ```
5. Refresh this manual from command help:
   ```bash
   multica --help
   multica issue --help
   multica issue create --help
   multica issue update --help
   multica issue dependency --help
   ```
6. Update `TODO.md` with the documentation change.

Do not include real auth tokens, workspace secrets, API keys, or private webhook secrets in examples.
