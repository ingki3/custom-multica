# TODO

## Completed

- [x] **In Review 이슈의 Agent task 상태 표시** — Board 카드의 task snapshot 조회 및 활성 task 배지를 `in_progress`뿐 아니라 `in_review`에도 적용. Review Agent task가 `running`이면 `Progress`, `queued`/`dispatched`이면 `Queued`로 표시하고 상태 선택 로직 회귀 테스트를 추가.
- [x] **Dev Agent → Sub Dev Agent fallback** — `failure_reason` taxonomy를 확장해 Codex auth/token 만료, rate/quota/context/model limit을 분류하고, agent별 fallback policy(`fallback_agent_id`, `fallback_failure_reasons`, `fallback_max_depth`)로 허용된 실패에서 fallback task를 생성하도록 구현. Same-provider `auth_expired`/`quota_exceeded` fallback 차단, fallback lineage/prompt/manual/타입/회귀 테스트 추가. 구현 계획: `.hermes/plans/2026-07-12_122032-dev-agent-fallback.md`.
- [x] **Claude 런타임 모델 목록 최신화** — Claude Code 2.1.185의 `claude models` 출력 기준으로 `claude-fable-5`(Claude Fable 5)와 `claude-opus-4-8`(Claude Opus 4.8)을 Claude runtime static catalog에 추가하고 Sonnet 4.6 기본값은 유지. Opus 4.8 비용 추정 매핑과 회귀 테스트를 추가했으며, 로컬 CLI 빌드/설치 및 데몬 재시작 후 `/api/runtimes/{claude}/models` 결과에 Fable 5/Opus 4.8/Sonnet 4.6이 표시되는 것을 확인.
- [x] **Antigravity `agy` 런타임 자동 감지** — `agy` provider를 추가해 데몬 `LoadConfig`가 `MULTICA_AGY_PATH`/`MULTICA_AGY_MODEL`을 읽고 PATH의 `agy` CLI를 런타임으로 등록하도록 구현. `agy --print` 기반 실행 backend, `agy models` 기반 동적 모델 조회, launch header, 회귀 테스트를 추가. 지원 CLI/환경변수 문서와 provider matrix를 12개 도구 기준으로 갱신하고, 로컬 `/opt/homebrew/bin/multica` 바이너리를 새 빌드로 교체한 뒤 데몬을 재시작해 status에 `agy`가 표시되는 것을 확인.
- [x] **Antigravity Planning Agent task-scoped auth token 정식 수정** — Migration 073으로 `task_token` 테이블 추가, sqlc query/generated code 추가, `mat_` task token 생성 helper 추가. daemon task claim 응답에 `auth_token`을 mint/persist/반환하고, middleware에서 `mat_` 토큰을 agent/task/workspace actor header로 인증하도록 구현. task 완료/실패/취소 시 token revoke를 추가하고, claim/auth 회귀 테스트 및 targeted backend tests를 통과.
- [x] **Multica CLI issue dependency subcommand** — `multica issue dependency` 하위에 `list`, `add --direction prerequisite|next`, `requires`, `then-runs`, `remove/delete/rm` 명령을 추가. 기존 `issue create/update --requires/--then-runs`와 동일한 `/api/issues/{id}/dependencies` API를 사용하고, table/json 출력 및 도움말/회귀 테스트를 추가. 로컬 설치 CLI도 새 바이너리로 교체하여 즉시 사용 가능하게 배포.
- [x] **Issue 상태 변경 Webhook 지원** — Migration 072로 `issue_status_transition`, `workspace_webhook`, `webhook_delivery` 추가. 모든 issue status 변경을 `from_status` → `to_status` 전환으로 기록하고 `issue.status_changed` 이벤트를 발행하도록 연결. workspace별 webhook CRUD/secret rotate/test/delivery retry API, HMAC 서명, retry dispatcher, 실행 가능한 payload(issue UUID/identifier, workspace slug, task/agent/project context), Settings Webhooks UI, core 타입/API를 추가.
- [x] **Hermes Generic Webhook 호환성 보강** — `X-Webhook-Signature`를 raw body 기반 HMAC-SHA256 hex로 전송하고, Hermes 이벤트 추출을 위해 payload에 `event_type`을 추가. retry dedupe 호환을 위해 `X-Request-ID`를 `X-Multica-Delivery`와 동일한 delivery id로 전송하도록 수정.
- [x] **Webhook Test Delivery payload 보정** — Settings Webhooks의 Test Delivery sample payload에도 실제 운영 이벤트와 동일하게 `event_type: issue.status_changed`를 포함하도록 수정하고 회귀 테스트를 추가.
- [x] **Docker 없이 셀프호스팅 지원** — `ensure-postgres.sh`가 네이티브 PostgreSQL을 우선 감지하도록 수정. `dev.sh`에서 Docker 필수 요구사항 제거. `Makefile`의 `db-up`/`db-down`/`db-reset`이 네이티브 PostgreSQL에서도 동작하도록 수정. `make selfhost-native` / `make selfhost-native-dev` 타겟 추가. `docker compose` vs `docker-compose` 자동 감지. `SELF_HOSTING.md`, `SELF_HOSTING_ADVANCED.md`, `README.md` 문서 업데이트.
- [x] **install.sh에서 Docker 없이 --with-server 지원** — `check_docker()`를 boolean 반환으로 변경. `check_native_prereqs()` 추가 (Go, Node, pnpm, PostgreSQL 검증). `setup_server_native()` 추가 (네이티브 빌드 및 실행, PID 파일 관리). `run_with_server()`가 Docker/네이티브 자동 분기. `run_stop()`이 PID 파일 기반 네이티브 프로세스 종료 지원.
- [x] **Bootstrap 토큰 자동 생성** — `MULTICA_BOOTSTRAP_EMAIL` 환경변수 설정 시 서버 시작 시 자동으로 admin 유저 + 1년 만료 PAT 토큰 생성. `server/cmd/server/bootstrap.go` 신규 파일. 토큰은 최초 1회만 생성되고 stderr에 출력. `.env.example`에 문서화.
- [x] **multica setup에서 기존 토큰으로 로그인 스킵** — `server/cmd/multica/cmd_setup.go`에 `tokenIsValid()` 함수 추가. `runSetupCloud()`/`runSetupSelfHost()`에서 유효한 토큰이 있으면 브라우저 로그인 건너뛰고 workspace만 자동 설정.
- [x] **Autopilot에 project_id 추가** — Autopilot 생성 시 프로젝트를 지정하면 `create_issue` 모드에서 자동 생성된 이슈가 해당 프로젝트에 속하도록 설정. Migration 065로 `project_id` 컬럼 복원. sqlc 쿼리, Go 핸들러(Create/Update), autopilot 서비스의 이슈 생성 로직(`dispatchCreateIssue`에서 `ap.ProjectID` 전달), 프론트엔드 타입 및 Autopilot 다이얼로그에 프로젝트 선택 드롭다운 추가.
- [x] **프로젝트별 Working Folder 설정** — `project` 테이블에 `working_folder TEXT` 컬럼 추가 (migration 064). sqlc 쿼리 및 Go 핸들러에 working_folder CRUD 반영. ClaimTask 응답에 project의 working_folder 전달. 데몬의 runTask에 3단계 WorkDir 우선순위 (WorkingFolder > PriorWorkDir > Prepare). `execenv.ReuseCustomFolder()` 함수 추가 (폴더 존재/쓰기 검증, context 파일만 주입, GC 자동 제외). 기존 CLAUDE.md 보존 로직 (`RuntimeConfigFilename()` 헬퍼). 프론트엔드 타입/생성 모달/상세 사이드바에 Working Folder 필드 추가.

## Backlog

- [x] **런타임 지원 모델 외부 config 관리** — 현재 설치된 runtime CLI가 지원하는 모델을 재확인해 별도 JSON 텍스트 config로 관리하고, runtime 모델 목록을 설정/조회할 때마다 파일을 다시 읽어 daemon 재시작이나 바이너리 재빌드 없이 변경사항이 반영되도록 구현. config 경로 override, 기본 파일 생성, 검증 및 hot-reload 회귀 테스트와 운영 문서를 포함. (`feat/runtime-model-config`)

- [ ] **[Upstream Release Review 2026-07-12] v0.3.34~v0.3.43 및 upstream/main 후속 커밋 수동 포팅 후보 정리** — GitHub Releases(`https://github.com/multica-ai/multica/releases`) 기준 최신 릴리즈 `v0.3.43`, fetch 기준 upstream 위치 `v0.3.43-27-g3bd69bdaa`까지 확인. 현재 fork `dev`는 `02d12797a`이며, 로컬 WIP(`server/pkg/agent/models.go`, `server/pkg/agent/models_test.go`, `.hermes/`)가 있어 broad merge 금지. 아래 후보는 **기능 단위 수동 포팅**으로 진행하고, migration 번호는 fork 흐름에 맞게 재채번한다.
  - **포팅 원칙**:
    1. `upstream/main`을 통째로 merge/rebase하지 않는다. custom fork에는 task-scoped auth token, working folder/worktree, issue dependency, workspace MCP registry, Dev Agent fallback 등 충돌 가능성이 큰 기능이 많다.
    2. PR은 작게 나눈다: `안전 패치`, `runtime 안정성`, `task/comment delivery`, `daemon exec isolation`, `UI/전략 기능` 순서.
    3. SQL 변경은 `make sqlc` 또는 `cd server && go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate` 후 generated diff를 검토한다.
    4. CLI 동작 변경은 `multica-cli-manual.md`와 public CLI docs 동기화 여부를 확인한다.
    5. 검증은 최소 `cd server && go test ./...`, frontend 영향 시 `pnpm typecheck && pnpm test`, CLI 배포 시 `make build && install -m 0755 server/bin/multica /opt/homebrew/bin/multica && multica --version`.
  - **1차: 작은 안전 패치 묶음(낮은 충돌, 먼저 적용 권장)**:
    1. `9f775db16` — `fix(cli): stop pflag from garbling login --token help output (MUL-4410)`; 파일: `server/cmd/multica/cmd_login.go`, `cmd_auth_test.go`. CLI help 품질 개선, 위험 낮음.
    2. `3fdcdb1a3` — `fix(cli): retry transient assignee resolver fetches`; 파일: `server/cmd/multica/cmd_issue.go`, `cmd_issue_test.go`. agent/CLI issue assign 안정성 개선.
    3. `b6adf23f9` — `feat(api): emit Content-Length header on JSON responses`; 파일: `server/internal/handler/handler.go`, `server/cmd/server/health.go`. 프록시/클라이언트 호환성 개선.
    4. `932bbf2bb` — `fix(editor): guard Mod-Enter submit against open IME composition`; 파일: `packages/views/editor/extensions/submit-shortcut.ts`. 한글/일본어 IME 입력 중 accidental submit 방지.
    5. `78591f602` — `test(server): cap pkg/agent test concurrency under -race`; race/full test 안정화 후보.
  - **2차: Codex/Claude/Antigravity runtime 안정성 묶음(높은 우선순위)**:
    - **상태 (2026-07-12)**: fork 구조에 맞춘 수동 포팅 완료. ChatGPT.app/Codex.app 탐색, hook wrapper 회피, Codex model-catalog 복사·동적 model/reasoning discovery와 GPT-5.6 fallback, turn-completed/EOF race, Claude stale-resume/root-sudo preflight, agy current-turn transcript recovery를 회귀 테스트와 함께 반영. 로컬에 아직 `thinking_level` 저장/실행 옵션이 없어 `8e0fbecab`의 effort persistence/validation 부분은 별도 schema/UI 작업으로 남김.
    1. `c7002e3b3` — `fix(daemon): detect Codex CLI under ChatGPT.app on macOS`; 파일: `server/internal/daemon/config.go`, `config_test.go`. macOS ChatGPT.app 내 Codex CLI 감지.
    2. `f7ca045fb` — `feat(daemon): discover Codex model and reasoning catalog dynamically`; 파일: `server/pkg/agent/models.go`, `thinking.go`, `server/internal/handler/agent.go`, `server/cmd/multica/cmd_agent.go`, core agent types. 현재 fork의 static Codex catalog WIP와 충돌 가능성이 높으므로 이 WIP와 함께 정리.
    3. `6b980a8e7` / `8e0fbecab` — Codex `gpt-5.6` sol/terra/luna 모델 및 exact alias/empty-model effort validation. 현재 fork의 `gpt-5.5`/fallback classifier와 함께 catalog policy 결정 필요.
    4. `65269ef92` — `fix(daemon): copy Codex model catalog into task home`; 파일: `server/internal/daemon/execenv/codex_home.go`. task sandbox에서 Codex model catalog 누락 방지.
    5. `1ff99e5a` — `fix: honor completed codex turns during process eof races`; 파일: `server/pkg/agent/codex.go`. EOF race로 성공 turn을 실패 처리하는 문제 방지.
    6. `521052a0` — `fix(daemon): recover stale Claude resume sessions`; 파일: `server/pkg/agent/claude.go`. stale Claude session 복구.
    7. `e36c0cd4` — `fix: preflight Claude root/sudo launches with an actionable error`; root/sudo 실행 전 명확한 에러 제공.
    8. `d4f57aff` — `Fix daemon agent discovery around hook wrappers`; wrapper 경유 provider discovery 안정화.
    9. `8b529efc` — `Fix recovered Antigravity transcript messages`; Antigravity transcript recovery 개선.
  - **3차: task/comment/chat delivery 안정성 묶음(매우 중요, 충돌 큼)**:
    1. `75695a2e` — `fix(comments): guarantee at-least-once processing of user comments (MUL-4195)`; 파일: `server/internal/handler/comment.go`, `daemon.go`, `server/internal/service/task.go`, migrations `agent_task_coalesced_comments`. user comment 누락 방지.
    2. `bf161f2f` — `fix(tasks): preserve merged comment delivery`; comment coalescing 후 delivery 보존.
    3. `4db1abe1` — `fix(comment): compensate dropped agent→agent @mentions in completion reconcile`; A2A mention 누락 보정.
    4. `cc3daaf3` — `fix: scope claim-time comment fetch to workspace + guard --attachment paths (MUL-4252)`; multi-tenant scoping 및 attachment path guard.
    5. `86c3f305` — `fix(server): keep originator on agent-created issues so A2A mentions stay authorized (MUL-4305)`; fallback/sub-agent issue 생성 권한과 직접 연관.
    6. `cb87dd10`, `f599333f`, `3c417ea6` — direct-chat input ownership, per-root-thread coalesced reply routing, authz/order correction. Chat V2와 얽혀 있을 수 있어 별도 설계 검토 후 적용.
    7. `7a405fd1` — `fix(daemon): keep the task transcript ordered and complete`; daemon transcript ordering/complete 보존. fallback prompt context와 충돌 검토 필요.
  - **4차: daemon execution isolation / runtime recovery 묶음(working folder와 충돌 주의)**:
    1. `aecd47b5` — `fix(daemon): mark workspaces root so escaped subprocesses still fail closed`; 파일: `server/internal/daemon/execenv/context.go`, `execenv.go`.
    2. `e002ee5a` — `fix(daemon): isolate agent temp dirs`; task temp dir 격리.
    3. `528d3c7f` — `fix: use short task temp dirs for agent env`; 긴 path로 인한 agent env 문제 완화.
    4. `101cc29e` — `fix(daemon): time out repo cache git commands`; repo cache hang 방지.
    5. `a69d969f` — `fix(sweeper): gate running-task wall clock on runtime liveness`; runtime이 죽은 상태에서 running task wall-clock 처리 개선. fallback/retry 정책과 함께 설계 필요.
    6. `6c3b79db` — `feat(daemon): bound daemon.log size with rotation (MUL-4330)`; 장기 실행 local/daemon 운영에 중요, 상대적으로 단독 포팅 가능.
  - **5차: security/guardrail 후보**:
    1. `3b7eafc3` — `fix(cli): reject --description-file/--content-file paths outside the workdir (MUL-4252)`; CLI agent 사용 시 workspace 밖 파일 접근 방지. `multica-cli-manual.md` 업데이트 필요.
    2. `0c4c3ff0` — `fix(cli): prevent daemon-managed CLI from silently using user tokens (MUL-3922)`; task-scoped `mat_` token 정책과 함께 비교. agent task가 사용자 토큰을 조용히 쓰는 문제 방지.
    3. `4f371c5c` — `fix: expose mcp config for supported providers`; MCP support matrix 정확화.
  - **6차: realtime/search/migration 운영 안정성**:
    1. `e3e3e7e` — `fix(realtime): bounded replay window for ShardedStreamRelay on restart`; 우리 repo에 `ShardedStreamRelay`가 이미 있으므로 반영 여부 diff 확인부터.
    2. `f30898f3` — `MUL-4299: guard migration numbering`; fork에서 migration 충돌을 자주 겪으므로 guard 도입 검토.
    3. `359ef61d` — `fix(search): pg_trgm index fallback + statement_timeout guard`; search 성능/장애 격리.
    4. `75e8bd5b` — `fix: make release index migrations concurrent`; index migration 운영 리스크 완화. upstream migration 번호 그대로 사용 금지.
  - **7차: product/UX 전략 후보(별도 큰 milestone)**:
    1. `a51ab4d5` — `feat(chat): Chat V2 — first-class IM-style Chat tab (MUL-4171)`; web/desktop/core/views/server/migrations 전체를 건드리는 대형 기능. 즉시 포팅보다 별도 설계 필요.
    2. `e6e63e6` — LLM-generated chat session titles; Chat V2와 묶어서 검토.
    3. `a1409828` — Agent Skills/MCP capabilities redesign; 우리 fork의 workspace MCP registry와 충돌 가능성이 커서 설계 비교 필요.
    4. `05d92985` / `c56f0816` / `3c3a3fed` — runtime local skills 권한/보존/Add to agent 개선. MCP/skills redesign과 묶어서 검토.
    5. `fd58e13b`, `2affa34f`, `f8c4c881` — custom runtime names, searchable/machine-grouped/two-level runtime picker, runtime model+effort hover card. 다중 runtime 환경 UX 개선.
    6. `519d2aef` — CLI issue ordering(`issue reorder`, `--position`, `--sort/--direction`); issue dependency 흐름과 결합 가치 있음.
    7. `c377d7fb`, `c4b116ec` — scoped label management 및 create issue dialog label entry.
    8. `835b1d5e`, `bcad2edc`, `8c3745dc` — issue detail thread minimap, Cmd+F in-page find, Show sub-issues toggle.
  - **8차: editor/attachment 품질 후보**:
    1. `3f02083f` — Linear-style issue identifier autolink.
    2. `cac2965d`, `0ffb5f68` — markdown URL autolink parse tree/trailing delimiter fix.
    3. `5ed381a9` — comment attachment URL resolution.
    4. `30d3aca6` — HTTP Range resume on proxy download.
    5. `c84a939c` — all attachment upload buttons multi-file selection.
    6. `c3a33fff` — inline data-URI image rendering.
  - **후순위/선택 후보**:
    1. Slack/Lark/Composio 계열(`4217de`, `fd3216`, `240ec4`, `159d9b`, `ccacce`, 등)은 현재 fork 핵심 운영 경로가 아니면 보류.
    2. mobile 관련 변경은 우리 fork에서 mobile을 적극 운영하지 않는다면 보류.
    3. avatar/cropper/surface system UI redesign(`c6783efd`, `f4de0948`, `4efcfb96`, `ca46fdb4`)은 시각 품질 개선이지만 충돌 범위가 넓어 안정성 포팅 이후 검토.
  - **주요 충돌 예상 파일/영역**:
    1. `server/pkg/db/queries/agent.sql`, generated `agent.sql.go`, `models.go` — fallback/task-token/chat/comment/runtime liveness 변경이 모두 겹칠 수 있음.
    2. `server/internal/service/task.go` — retry/fallback/comment delivery/chat input ownership 충돌 예상.
    3. `server/internal/daemon/daemon.go` — working folder, task-scoped auth, provider discovery, transcript ordering, log rotation 충돌 예상.
    4. `server/internal/handler/daemon.go`, `comment.go`, `chat.go`, `issue.go` — originator/comment/claim/chat delivery 변경 충돌 예상.
    5. `packages/core/types/agent.ts` — fallback fields와 upstream runtime/capability/model/thinking fields 충돌 가능.
    6. `packages/views/chat/*` — Chat V2와 현재 chat-window/fallback context UI 충돌 가능.
  - **권장 실행 순서**:
    1. `safe-upstream-patches` PR: CLI help/retry, Content-Length, IME submit guard, test concurrency cap, daemon log rotation.
    2. `runtime-stability-sync` PR: Codex dynamic catalog/ChatGPT.app detection/EOF race, Claude stale resume/root preflight, Antigravity transcript recovery.
    3. `task-comment-delivery-sync` PR: at-least-once comments, merged comment delivery, A2A reconcile, originator preservation, claim-time workspace scoping.
    4. `daemon-exec-isolation-sync` PR: workspace root marker, temp dir isolation, short temp dirs, repo cache timeout, runtime liveness sweeper.
    5. `runtime-ui-capabilities-sync` PR: runtime picker/custom names/capabilities/skills/MCP redesign은 별도 설계 후.
    6. `chat-v2-sync`는 마지막에 별도 milestone로 진행.
- [ ] **[Upstream Sync] 안정성 → 데이터 보존 → Cursor managed MCP → workspace repo registry 순차 반영** — 코드 레벨 검토 결과는 `docs/upstream-sync-candidates-2026-06-13.md`에 저장. 1차 안정성 후보 중 daemon workdir provisioning race(`9439a85aa`), stale resume session drop(`8151f60c6`), ACP stale session clear(`6acca84c2`), Codex cached input usage normalization(`5b7eb9ad2`), setup self-host `MULTICA_SERVER_URL` 반영(`42251b42f`)을 `feat/upstream-stability-sync`에서 포팅 완료하고 targeted Go tests 통과. 2차 데이터 보존 후보: attachment `markdown_url` 계열, issue description flush, create/quick-create attachment binding, chat stop/send recovery. 3차 Cursor managed MCP(`f415099c4`). 4차 workspace repo registry CLI/API(`7db3e507d`). broad merge 금지, 기능 단위 수동 포팅 우선.
- [x] **Working Folder 동시 접근 정책** — 같은 프로젝트에 여러 에이전트가 동시에 할당되면 같은 폴더에서 작업하게 됨. 하이브리드 정책 구현: git 레포인 경우 `.multica_worktrees/{taskID}/`에 per-task worktree 자동 생성으로 격리, non-git 폴더인 경우 태스크를 큐로 되돌려 직렬화. 데몬 내 `workingFolderTasks map[string]int`로 폴더별 활성 태스크 수 추적. `RequeueTask` API 엔드포인트 추가. 단일 태스크 시 기존 동작 변경 없음.
- [ ] **Working Folder에 생성되는 .agent_context/ 정리** — 에이전트 작업 후 `.agent_context/` 폴더가 사용자 프로젝트에 남음. 자동 정리 정책 또는 `.gitignore` 자동 추가 검토.
- [ ] **CLI 기반 로그인 (브라우저 없이)** — 터미널에서 이메일 + 인증코드를 직접 입력하여 토큰 발급. 헤드리스 환경 지원.
- [x] **워크스페이스 MCP 서버 공유 관리** — 현재 MCP 서버는 각 에이전트마다 개별 설정(`agent.mcp_config`)하는 구조. Migration 067로 `workspace_mcp_server` + `agent_mcp_server` 테이블 추가. REST API (CRUD + 에이전트 연결), ClaimTask에서 공유 + 개별 MCP 설정 자동 병합, 프론트엔드 타입 + API 클라이언트 구현. Skill은 "워크스페이스에 공통 등록 → 에이전트에서 선택" 패턴인데 MCP는 이 패턴이 없어서, 같은 MCP 서버를 여러 에이전트에 설정하려면 동일 JSON을 반복 입력해야 함.
  - **개선 방향**: Skill과 동일한 2-tier 모델 도입
    1. **워크스페이스 레벨 MCP 서버 레지스트리**: `workspace_mcp_server` 테이블에 공통 MCP 서버를 등록 (name, transport, command/url, args, env)
    2. **에이전트별 선택**: `agent_mcp_server` 조인 테이블로 에이전트가 사용할 MCP 서버를 선택/해제. 에이전트별 env 오버라이드 지원 (API 키 등 에이전트마다 다를 수 있는 값).
    3. **UI**: Settings 페이지에 "MCP Servers" 관리 섹션 추가 (CRUD). 에이전트 상세 MCP 탭에서 워크스페이스 등록 서버 선택 체크박스 + 개별 설정 병행 가능.
    4. **데몬 연동**: 태스크 claim 시 에이전트의 개별 `mcp_config` + 워크스페이스 공유 MCP 서버를 병합하여 전달.
  - **하위 호환**: 기존 에이전트별 `mcp_config` 필드는 유지 — 개별 설정과 공유 설정이 병합됨.
- [x] **[Upstream] Auth Token TTL** (MUL-2371) — `AUTH_TOKEN_TTL` 환경변수로 인증 토큰 만료 시간 설정. 기본 30일, Go duration 문자열 지원. `auth/cookie.go`에 `AuthTokenTTL()` 함수 + `parseAuthTokenTTL()` 파서 추가. `SetAuthCookies`에서 동적 TTL 사용.
- [x] **[Upstream] Parent/Sub-Issue Protocol** (MUL-2338) — 에이전트에게 부모-자식 이슈 관계 워크플로우 교육. runtime_config.go에 "Parent / Sub-issue Protocol" brief 섹션 추가: 자식 완료 시 부모에 보고, sub-issue 생성 시 status 전략(todo vs backlog).
- [x] **[Upstream] User Profile Description** (MUL-2406) — 사용자 프로필 설명을 에이전트 브리핑에 주입. Migration 068로 user.profile_description 추가. runtime_config.go에 "Requesting User" 섹션 + sanitizeNameForBriefMarkdown 헬퍼. ClaimTask에서 runtime owner 프로필 조회 및 전달.
- [x] **[Upstream] Project Gantt View** (MUL-1881) — Migration 069로 issue.start_date 추가. Go 핸들러 start_date CRUD 구현. Gantt 뷰 컴포넌트(~220줄) SVG/HTML 기반. 프로젝트 상세 Board/List/Gantt 토글. 줌(day/week/month), show-completed 지원.
- [ ] **[Upstream] Issue Prefix Edit** (MUL-2369) — 워크스페이스 이슈 접두사 변경 UI. Settings → General에서 편집.
- [ ] **[Upstream] Agent Thinking Level** (MUL-2339) — 에이전트별 thinking level 설정 (Claude/Codex). 마이그레이션 필요, ~2000줄 대규모 변경.
- [ ] **Admin 2.0 제작 계획 수립** (BIZ-108) — admin.pen 디자인 파일 점검 → Design Agent가 부족한 부분 보완 → Dev Agent가 서브 이슈 분해 → `--requires`/`--then-runs`로 순서 설정하여 순차 개발 진행. 3단계 워크플로: 1) Design Agent 디자인 점검/보완 2) Dev Agent 서브이슈 분해 3) 서브이슈 순차 실행.
- [x] **이슈 간 의존성 기반 실행 순서 제어** — 이슈에 선행 조건(prerequisites)과 후속 이슈(next issues)를 설정하여, 선행 이슈가 모두 Done이 되면 후속 이슈가 자동으로 In Progress로 전환되어 순서대로 개발이 진행되도록 한다. Migration 066으로 type 제약 변경 + 인덱스 + unique 추가. `issue_dependency.sql`에 CRUD + 순환검사 쿼리. `ClaimAgentTask`에 선행이슈 검사 추가. `ActivateNextIssues`로 Done 시 후속 이슈 자동 활성화. REST API (`GET/POST/DELETE /api/issues/{id}/dependencies`) + 프론트엔드 타입 + API 클라이언트 구현.
  - **핵심 개념**:
    1. **선행 이슈 (Prerequisites)**: 해당 이슈를 실행하기 위해 먼저 Done 상태가 되어야 하는 이슈 목록. 선행 이슈가 모두 Done이 아니면 에이전트에게 태스크가 디스패치되지 않음.
    2. **후속 이슈 (Next Issues)**: 해당 이슈가 Done이 되었을 때 자동으로 Todo → In Progress로 이동시킬 이슈 목록. 후속 이슈의 선행 조건이 모두 충족되면 즉시 실행 시작.
  - **동작 흐름**: `A(Done) → B(자동 In Progress) → B(Done) → C(자동 In Progress) → ...` 식으로 이슈 체인이 순서대로 자동 실행됨.
  - **현재 상태**: `issue_dependency` 테이블이 초기 마이그레이션(001_init.up.sql)에 스키마만 존재하고, 쿼리/API/UI가 전혀 구현되지 않은 데드 코드 상태. 태스크 디스패치는 우선순위 큐(priority DESC, created_at ASC) 기반이며 의존성 검사 없음.
  - **구현 범위**:
    1. **API/쿼리 계층**: `issue_dependency` 테이블에 대한 CRUD 쿼리(sqlc) 및 REST 엔드포인트 (`POST/DELETE /api/issues/{id}/dependencies`) 추가. 관계 타입은 `prerequisite`(선행)와 `next`(후속) 두 가지.
    2. **UI**: 이슈 상세 사이드바에 의존성 관리 섹션 추가 — "선행 이슈"와 "후속 이슈" 목록 표시, 이슈 검색/선택으로 추가/삭제.
    3. **태스크 디스패치 연동**: `ClaimAgentTask` SQL 쿼리에 선행 이슈 검사 조건 추가 — `prerequisite` 타입의 의존 이슈가 모두 `done` 상태인 경우에만 태스크 claim 가능. 미충족 시 해당 태스크를 스킵하고 다음 태스크를 claim.
    4. **이슈 완료 시 후속 이슈 자동 실행**: 이슈가 Done이 되면 해당 이슈를 `prerequisite`로 가진 후속 이슈들을 확인하고, 모든 선행 조건이 충족된 후속 이슈를 자동으로 `in_progress`로 전환 + 에이전트 태스크 생성.
    5. **순환 의존성 방지**: 의존성 추가 시 순환 참조(A→B→C→A) 검증 로직 필요.
  - **양방향 자동 설정**: 한쪽만 설정하면 반대쪽은 자동으로 설정된다.
    - B를 A의 후속 이슈로 지정 → A는 자동으로 B의 선행 이슈가 됨
    - A를 B의 선행 이슈로 지정 → B는 자동으로 A의 후속 이슈가 됨
    - DB에는 단일 레코드(`depends_on_issue_id=A, issue_id=B`)만 저장하고, 조회 시 방향에 따라 해석
  - **설정 시 검증**: 의존성 추가 후 즉시 순환(루프) 검사를 수행. A→B→C→A 같은 순환이 감지되면 추가를 거부하고 에러 반환. 검증 통과 후에만 확정.
  - **기존 스키마**: `issue_dependency(id UUID, issue_id UUID FK, depends_on_issue_id UUID FK, type TEXT)` — 단일 레코드로 양방향 관계 표현. `depends_on_issue_id`가 선행, `issue_id`가 후속.
