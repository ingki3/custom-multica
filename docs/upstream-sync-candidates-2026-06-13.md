# Upstream Sync Candidates — 2026-06-13

Source upstream: https://github.com/multica-ai/multica

Local fork path: `/Users/simplist/Dev/multica-main`

Inspection date: 2026-06-13

Scope: recent upstream changes inspected at code level. This file is a planning/reference note only; it does not mean the items have been implemented in this fork.

## Current upstream snapshot

- Upstream remote: `upstream` → `https://github.com/multica-ai/multica.git`
- Latest inspected upstream/main: `34d4cd3a2`
- Upstream describe: `v0.3.21-9-g34d4cd3a2`
- Recent release tag: `v0.3.21` on 2026-06-12
- Local branch at inspection time: `dev`
- Important caveat: the fork and upstream have diverged significantly. Prefer feature-by-feature manual porting over a broad merge or blind cherry-pick.

## Proposed implementation order

1. Stability and runtime correctness
2. Data preservation, editor, attachment, and chat reliability
3. Cursor managed MCP
4. Workspace repo registry
5. Optional/later product-direction features

---

## 1. Stability and runtime correctness

### 1.1 Daemon workdir provisioning race

- Commit: `9439a85aa` — `MUL-3242: fix daemon workdir provisioning race`
- Main files:
  - `server/internal/daemon/daemon.go`
  - `server/internal/daemon/workdir_race_test.go`

Code-level summary:
- Moves `StartTask` so a task is marked running only after the workdir/env is prepared.
- Prevents the server/UI from observing `running` before the on-disk workdir exists.
- Extends active env-root guarding through completion metadata writes to reduce GC races.

Why it matters here:
- High priority because this fork has working folder, worktree, daemon, and agent lifecycle customizations.
- It directly reduces race conditions after restarts, task handoff, and GC.

Risk:
- Medium/high. `server/internal/daemon/daemon.go` is likely fork-diverged. Manual porting is safer than cherry-pick.

### 1.2 Drop stale resume session when workdir is not reused

- Commit: `8151f60c6` — `fix(daemon): drop stale resume session when workdir is not reused (#4027)`
- Main files:
  - `server/internal/daemon/daemon.go`
  - `server/internal/daemon/daemon_test.go`

Code-level summary:
- Adds logic equivalent to `gateResumeToReusedWorkdir`.
- Keeps `PriorSessionID` only when the prior workdir is actually reused.
- If workdir was GC'd/recreated/reprepared, clears stale session id and treats the run as a fresh start.

Why it matters here:
- Prevents persistent failure loops where a CLI backend tries to resume a session tied to a no-longer-valid cwd.

Risk:
- Medium. Needs careful placement in `runTask`/execenv prepare/reuse flow.

### 1.3 ACP stale session clear for Hermes/Kimi/Kiro

- Commit: `6acca84c2` — `fix(agent): clear stale session id when a resumed ACP session is gone [MUL-3216] (#4015)`
- Main files:
  - `server/pkg/agent/hermes.go`
  - `server/pkg/agent/kimi.go`
  - `server/pkg/agent/kiro.go`
  - related tests

Code-level summary:
- Detects structured ACP JSON-RPC `Session not found` style errors.
- For resumed sessions that no longer exist, returns an empty `SessionID` so daemon fallback can retry with a fresh session.
- Handles errors in both `session/prompt` and `session/set_model` paths.

Why it matters here:
- Reduces stuck task runs after external session cleanup, daemon restart, or workdir replacement.

Risk:
- Medium. Check compatibility with local Hermes generic compatibility changes.

### 1.4 Codex cached input usage normalization

- Commit: `5b7eb9ad2` — `fix: normalize codex cached input usage (#4083)`
- Main files:
  - `server/pkg/agent/codex.go`
  - `server/pkg/agent/codex_test.go`

Code-level summary:
- Handles Codex usage where `input_tokens` includes `cached_input_tokens`.
- Persists mutually exclusive buckets:
  - `InputTokens = max(input_tokens - cached_input_tokens, 0)`
  - `CacheReadTokens = cached_input_tokens`
- Applies both to live JSON-RPC usage and session JSONL parsing.

Why it matters here:
- Improves usage/cost accounting accuracy.

Risk:
- Low/medium. Small targeted patch.

### 1.5 Self-host setup honors MULTICA_SERVER_URL

- Commit: `42251b42f` — `fix(cli): honor MULTICA_SERVER_URL in setup self-host (#3912) (#3938)`
- Main files:
  - `server/cmd/multica/cmd_setup.go`
  - `server/cmd/multica/cmd_setup_test.go`

Code-level summary:
- Ensures `multica setup self-host` respects `MULTICA_SERVER_URL`.

Why it matters here:
- High value for local/self-host workflows in this fork.

Risk:
- Low/medium.

### 1.6 DB migration lock and index hardening

- Commits:
  - `2e0b0bb77` — drop FK on `agent_task_queue.initiator_user_id`
  - `99afb82c5` — add index on `"user".created_at`
- Main files:
  - `server/migrations/117_*`
  - `server/migrations/118_*`
  - `server/migrations/119_user_created_at_index.*`

Code-level summary:
- Removes a hot FK lock from `agent_task_queue.initiator_user_id`.
- Adds an index on `user.created_at`.

Why it matters here:
- Improves operational DB safety under real load.

Risk:
- Medium. Migration numbers will likely need renumbering in this fork. `CREATE INDEX CONCURRENTLY` must be compatible with this repo's migration runner.

---

## 2. Data preservation, editor, attachment, and chat reliability

### 2.1 Attachment markdown_url and durable URL handling

- Commits:
  - `abf99eb70` — server-driven `markdown_url` + legacy compat
  - `619c4c495` — bind description uploads via `contentReferencesAttachment`
  - `2754b7d7d` — render description images with CDN URL
  - `3e892a359` — prefer locally served `/uploads/<key>` over auth-gated download endpoint
- Main files:
  - `server/internal/handler/file.go`
  - `packages/core/types/attachment.ts`
  - `packages/core/api/schemas.ts`
  - `packages/core/hooks/use-file-upload.ts`
  - `packages/core/types/attachment-url.ts`
  - `packages/views/editor/attachment.tsx`
  - `packages/views/editor/attachment-preview-modal.tsx`
  - `packages/views/editor/content-editor.tsx`
  - `packages/views/issues/components/issue-detail.tsx`

Code-level summary:
- Adds durable `markdown_url` to attachment responses.
- Uses server-derived stable URLs for markdown instead of expiring signed download URLs.
- Improves Desktop/Electron/mobile image rendering.
- Binds only attachments actually referenced by content.

Why it matters here:
- Very high value for preventing broken images and orphaned attachments in issue descriptions, comments, and quick-create flows.

Risk:
- High. Server response shape, core schema/types, upload hook, editor rendering, and issue detail save logic must be ported as a coherent set.

### 2.2 Flush issue description edits on close

- Commit: `ef08d8584` — `MUL-3254: flush issue description edits on close (#4082)`
- Main files:
  - `packages/views/editor/content-editor.tsx`
  - `packages/views/issues/components/issue-detail.tsx`
  - related tests

Code-level summary:
- Adds opt-in `flushPendingOnUnmount` behavior to `ContentEditor`.
- Uses it for issue description editing so the final debounced edit is not lost when the view/modal closes.

Why it matters here:
- Prevents user-visible data loss, especially after image paste + immediate close.

Risk:
- Medium. Must keep opt-in behavior to avoid resurrecting drafts in create/comment composers.

### 2.3 Pasted image draft rendering and create-issue attachment draft preservation

- Commit: `fa1504186` — `MUL-3254: fix pasted image draft rendering in desktop (#4066)`
- Main files:
  - `packages/core/issues/stores/draft-store.ts`
  - `packages/views/modals/create-issue.tsx`
  - `packages/views/editor/content-editor.tsx`
  - `packages/views/editor/attachment.tsx`
  - `packages/views/editor/attachment-preview-modal.tsx`

Code-level summary:
- Adds attachment records to issue draft store.
- Avoids persisting expiring signed URLs in draft state.
- Backfills preview URLs from current session uploads where possible.
- Prunes attachment records no longer referenced by draft markdown.

Why it matters here:
- Helps Desktop/create issue flows preserve pasted images across close/reopen.

Risk:
- Medium/high. Depends on attachment URL foundation above.

### 2.4 Quick-create attachment binding

- Commit: `c8ab73d38` — `MUL-3244: Bind quick-create attachments to created issues (#4062)`
- Main files:
  - `packages/core/api/client.ts`
  - `packages/views/modals/quick-create-issue.tsx`
  - `server/cmd/multica/cmd_issue.go`
  - `server/internal/handler/issue.go`
  - `server/internal/service/task.go`
  - `server/internal/daemon/types.go`

Code-level summary:
- Adds `attachment_ids` to quick-create request/context.
- Carries quick-create attachment ids through daemon/task payloads.
- Binds attachments to the created issue.

Why it matters here:
- Prevents pasted/dropped quick-create attachments from becoming orphaned.

Risk:
- High. Touches issue creation, task service, CLI, daemon payload, and frontend quick-create.

### 2.5 Chat stop/send recovery

- Commit: `d2a03b8ed` — `Fix chat stop and send recovery (#4060)`
- Main files:
  - `packages/core/api/client.ts`
  - `packages/core/realtime/use-realtime-sync.ts`
  - `packages/views/chat/components/chat-input.tsx`
  - `packages/views/chat/components/chat-window.tsx`
  - `server/internal/handler/chat.go`
  - `server/internal/service/task.go`

Code-level summary:
- Backfills created `task_id` to the user chat message.
- Uses cancel restore payloads so stopped sends can recover the user's input.
- Avoids ghost assistant snapshots when cancellation happens before assistant output.

Why it matters here:
- Improves agent chat reliability and reduces lost typed input.

Risk:
- Medium/high due to chat/task service divergence.

### 2.6 Small editor/issue fixes worth early adoption

- `04a067770` — keep dollar amounts literal in editor
  - `packages/views/editor/extensions/math.tsx`
- `f2ba3c8f1` — wrap tables in `tableWrapper` for local horizontal scroll
  - `packages/views/editor/extensions/index.ts`
- `0985bad9f` — render thread replies in chronological order
  - `packages/views/issues/components/thread-utils.ts`
- `7d28b5a04` — remove duplicate emoji reaction entry from comment header

Why it matters here:
- Good low-cost, low-risk polish and correctness fixes.

---

## 3. Cursor managed MCP

- Commit: `f415099c4` — `MUL-3263: support managed MCP config for Cursor (#4081)`
- Main files:
  - `server/internal/daemon/execenv/cursor_mcp.go`
  - `server/internal/daemon/execenv/execenv.go`
  - `server/internal/daemon/daemon.go`
  - `packages/core/agents/mcp-support.ts`
  - `packages/views/agents/components/agent-overview-pane.test.tsx`

Code-level summary:
- Adds Cursor to MCP-supported runtime list.
- When an agent has managed `mcp_config`, daemon writes `.cursor/mcp.json` into the workdir/project root.
- Creates task-scoped `CURSOR_DATA_DIR`.
- Creates Cursor trust/approval sidecar files:
  - `mcp-approvals.json`
  - `.workspace-trusted`
- Refuses to overwrite an existing `.cursor/mcp.json`.
- Blocks user override of `CURSOR_DATA_DIR`.

Why it matters here:
- This fork already has workspace MCP server registry. Cursor managed MCP is a natural next step so shared MCP config actually reaches Cursor tasks.

Open product/UX questions:
- What should happen when a user repo already has `.cursor/mcp.json`?
  - upstream fails closed.
  - alternatives: merge, backup, or expose a warning in UI.
- Should workspace-level MCP registry and agent-level MCP selections be rendered differently for Cursor because it writes project files?

Risk:
- Medium/high. Touches daemon execenv, workdir policy, and MCP UI/support metadata.

---

## 4. Workspace repo registry CLI/API

- Commit: `7db3e507d` — `feat(cli): manage workspace repo registry (#4067)`
- Main files:
  - `server/cmd/multica/cmd_repo.go`
  - `server/cmd/multica/cmd_repo_test.go`
  - `server/internal/handler/workspace.go`
  - `server/internal/handler/workspace_test.go`

Code-level summary:
- Adds CLI:
  - `multica repo list`
  - `multica repo add [url]...`
  - `multica repo remove|rm [url]...`
  - `--url`, `--description`, `--output table|json`
- Adds workspace update validation/normalization for `repos`:
  - trim URL
  - reject empty URL
  - validate http(s)/ssh git URL
  - dedupe URLs
  - trim description

Why it matters here:
- Useful with project working folders and agent context. It gives a workspace-level repository registry that can feed checkout, context, and future automation.

Risk:
- Medium. No DB migration, but changes workspace handler/API behavior. Existing invalid repo URLs would start failing validation.

---

## 5. Optional/later product-direction features

### 5.1 Agent Thinking Level

- Commits:
  - `2bec2221d` — backend/schema for per-agent `thinking_level`
  - `9d3b6e224` — inspector picker UI
  - `68270e238` — optimistic update/default semantics polish
- Main files:
  - `server/migrations/095_agent_thinking_level.*`
  - `server/internal/handler/agent.go`
  - `server/internal/daemon/daemon.go`
  - `server/pkg/agent/thinking.go`
  - `packages/views/agents/components/inspector/thinking-picker.tsx`
  - `packages/views/agents/components/inspector/thinking-prop-row.tsx`

Why it matters here:
- Already listed in this fork's TODO as `[Upstream] Agent Thinking Level`.

Risk:
- High. Migration + sqlc + handler + daemon + UI.

### 5.2 Issue Prefix Edit

- Commit: `6e0f7b0f3` — `feat(settings): allow editing workspace issue prefix (MUL-2369) (#2809)`
- Main files:
  - `packages/core/api/client.ts`
  - `packages/core/realtime/use-realtime-sync.ts`
  - `packages/views/settings/components/workspace-tab.tsx`

Why it matters here:
- Already listed in this fork's TODO as `[Upstream] Issue Prefix Edit`.

Risk:
- Medium. Settings UI and realtime invalidation changes.

### 5.3 Skill import conflict strategies

- Commit: `e4ec9dc42` — `MUL-2802: add skill import conflict strategies (#3997)`
- Main files:
  - `server/cmd/multica/cmd_skill.go`
  - `server/internal/handler/runtime_local_skills.go`
  - `packages/core/runtimes/local-skills.ts`
  - `packages/views/skills/components/runtime-local-skill-import-panel.tsx`

Why it matters here:
- Better skill lifecycle UX. Preserves skill IDs and agent bindings on overwrite.

Risk:
- High. Backend/API/store/UI/CLI are strongly coupled.

### 5.4 OpenClaw existing gateway

- Commit: `34d4cd3a2` — `feat(openclaw): support connecting to existing OpenClaw gateway (#3260)`
- Main files:
  - `packages/core/agents/openclaw-runtime-config.ts`
  - `packages/views/agents/components/tabs/runtime-config-tab.tsx`
  - `server/internal/daemon/openclaw_runtime_config.go`
  - `server/internal/daemon/execenv/openclaw_config.go`
  - `server/pkg/agent/openclaw.go`
  - `server/internal/handler/agent.go`

Why it matters here:
- Valuable if OpenClaw gateway routing is part of product direction.

Risk:
- High. Runtime config schema, token masking/preserve semantics, daemon wrapper config, and SSRF policy must be handled together.

### 5.5 CodeBuddy first-class CLI backend

- Commit: `4594c776e` — `feat(agent): add CodeBuddy as first-class CLI backend (#3186)`
- Main files:
  - `server/pkg/agent/codebuddy.go`
  - `server/pkg/agent/agent.go`
  - `server/pkg/agent/models.go`
  - `server/pkg/agent/thinking.go`
  - `server/internal/daemon/config.go`
  - `packages/views/runtimes/components/provider-logo.tsx`

Why it matters here:
- Useful only if CodeBuddy should be supported as a first-class backend.

Risk:
- Medium/high. Provider registry, metrics, model discovery, thinking/effort discovery, and CLI execution policy all involved.

---

## Recommended working plan

### Phase 1 — Stability

Target candidates:
1. `9439a85aa` daemon workdir provisioning race
2. `8151f60c6` stale resume session drop
3. `6acca84c2` ACP stale session clear
4. `5b7eb9ad2` Codex usage normalization
5. `42251b42f` self-host setup `MULTICA_SERVER_URL`
6. migration lock/index review, if migration numbering is clear

Suggested verification:
- `cd server && go test ./internal/daemon ./pkg/agent ./cmd/multica`
- targeted tests for changed handlers/CLI commands
- operational smoke test only if daemon behavior changes materially

### Phase 2 — Data preservation

Target candidates:
1. attachment `markdown_url` foundation
2. issue description flush on close
3. create issue draft pasted image preservation
4. quick-create attachment binding
5. chat stop/send recovery
6. small editor/issue fixes

Suggested verification:
- targeted Vitest for editor/issue/chat components
- targeted Go handler/service tests
- manual UI smoke for paste image → close/reopen → save

### Phase 3 — Cursor managed MCP

Target candidate:
- `f415099c4`

Suggested verification:
- daemon execenv tests
- agent overview/MCP support tests
- manual Cursor task with workspace MCP server selected

### Phase 4 — Workspace repo registry

Target candidate:
- `7db3e507d`

Suggested verification:
- `server/cmd/multica/cmd_repo_test.go`
- `server/internal/handler/workspace_test.go`
- manual CLI `multica repo list/add/remove` against local server

## Notes for future implementation

- Do not broad-merge upstream/main.
- Prefer small feature branches from `dev`.
- Preserve current fork improvements:
  - working folder behavior
  - worktree auto-commit/cancel
  - automatic issue status transitions
  - workspace MCP server registry
  - issue dependency ordering
  - webhooks
  - self-host/native PostgreSQL support
- Watch for migration number conflicts.
- Stage only files for the current phase.
