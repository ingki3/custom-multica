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
- [ ] **이슈 간 의존성 기반 실행 순서 제어** — 이슈 간 의존 관계(blocks/blocked_by/related)를 설정하고, 의존성이 충족된 이슈만 에이전트에게 디스패치되도록 태스크 큐를 개선.
  - **현재 상태**: `issue_dependency` 테이블이 초기 마이그레이션(001_init.up.sql)에 스키마만 존재하고, 쿼리/API/UI가 전혀 구현되지 않은 데드 코드 상태. 태스크 디스패치는 우선순위 큐(priority DESC, created_at ASC) 기반이며 의존성 검사 없음.
  - **구현 범위**:
    1. **API/쿼리 계층**: `issue_dependency` 테이블에 대한 CRUD 쿼리(sqlc) 및 REST 엔드포인트 (`POST/DELETE /api/issues/{id}/dependencies`) 추가.
    2. **UI**: 이슈 상세 사이드바에 의존성 관리 섹션 추가 — blocks/blocked_by/related 관계 추가/삭제, 의존 이슈 칩 표시.
    3. **태스크 디스패치 연동**: `ClaimAgentTask` SQL 쿼리에 의존성 검사 조건 추가 — `blocked_by` 타입의 의존 이슈가 모두 완료(done/cancelled) 상태인 경우에만 태스크 claim 가능. 미충족 시 해당 태스크를 스킵하고 다음 태스크를 claim.
    4. **이슈 완료 시 연쇄 실행**: 이슈가 완료되면 해당 이슈에 `blocked_by`로 의존하던 이슈들의 대기 중 태스크를 자동으로 활성화(재큐잉 또는 알림).
    5. **순환 의존성 방지**: 의존성 추가 시 순환 참조(A→B→C→A) 검증 로직 필요.
  - **기존 스키마**: `issue_dependency(id UUID, issue_id UUID FK, depends_on_issue_id UUID FK, type TEXT['blocks','blocked_by','related'])` — 그대로 활용 가능.
