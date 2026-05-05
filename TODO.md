# TODO

## Completed

- [x] **Docker 없이 셀프호스팅 지원** — `ensure-postgres.sh`가 네이티브 PostgreSQL을 우선 감지하도록 수정. `dev.sh`에서 Docker 필수 요구사항 제거. `Makefile`의 `db-up`/`db-down`/`db-reset`이 네이티브 PostgreSQL에서도 동작하도록 수정. `make selfhost-native` / `make selfhost-native-dev` 타겟 추가. `docker compose` vs `docker-compose` 자동 감지. `SELF_HOSTING.md`, `SELF_HOSTING_ADVANCED.md`, `README.md` 문서 업데이트.
- [x] **install.sh에서 Docker 없이 --with-server 지원** — `check_docker()`를 boolean 반환으로 변경. `check_native_prereqs()` 추가 (Go, Node, pnpm, PostgreSQL 검증). `setup_server_native()` 추가 (네이티브 빌드 및 실행, PID 파일 관리). `run_with_server()`가 Docker/네이티브 자동 분기. `run_stop()`이 PID 파일 기반 네이티브 프로세스 종료 지원.
- [x] **Bootstrap 토큰 자동 생성** — `MULTICA_BOOTSTRAP_EMAIL` 환경변수 설정 시 서버 시작 시 자동으로 admin 유저 + 1년 만료 PAT 토큰 생성. `server/cmd/server/bootstrap.go` 신규 파일. 토큰은 최초 1회만 생성되고 stderr에 출력. `.env.example`에 문서화.
- [x] **multica setup에서 기존 토큰으로 로그인 스킵** — `server/cmd/multica/cmd_setup.go`에 `tokenIsValid()` 함수 추가. `runSetupCloud()`/`runSetupSelfHost()`에서 유효한 토큰이 있으면 브라우저 로그인 건너뛰고 workspace만 자동 설정.
- [x] **Autopilot에 project_id 추가** — Autopilot 생성 시 프로젝트를 지정하면 `create_issue` 모드에서 자동 생성된 이슈가 해당 프로젝트에 속하도록 설정. Migration 065로 `project_id` 컬럼 복원. sqlc 쿼리, Go 핸들러(Create/Update), autopilot 서비스의 이슈 생성 로직(`dispatchCreateIssue`에서 `ap.ProjectID` 전달), 프론트엔드 타입 및 Autopilot 다이얼로그에 프로젝트 선택 드롭다운 추가.
- [x] **프로젝트별 Working Folder 설정** — `project` 테이블에 `working_folder TEXT` 컬럼 추가 (migration 064). sqlc 쿼리 및 Go 핸들러에 working_folder CRUD 반영. ClaimTask 응답에 project의 working_folder 전달. 데몬의 runTask에 3단계 WorkDir 우선순위 (WorkingFolder > PriorWorkDir > Prepare). `execenv.ReuseCustomFolder()` 함수 추가 (폴더 존재/쓰기 검증, context 파일만 주입, GC 자동 제외). 기존 CLAUDE.md 보존 로직 (`RuntimeConfigFilename()` 헬퍼). 프론트엔드 타입/생성 모달/상세 사이드바에 Working Folder 필드 추가.

## Backlog

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
