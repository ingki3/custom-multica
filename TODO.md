# TODO

## Completed

- [x] **Docker 없이 셀프호스팅 지원** — `ensure-postgres.sh`가 네이티브 PostgreSQL을 우선 감지하도록 수정. `dev.sh`에서 Docker 필수 요구사항 제거. `Makefile`의 `db-up`/`db-down`/`db-reset`이 네이티브 PostgreSQL에서도 동작하도록 수정. `make selfhost-native` / `make selfhost-native-dev` 타겟 추가. `docker compose` vs `docker-compose` 자동 감지. `SELF_HOSTING.md`, `SELF_HOSTING_ADVANCED.md`, `README.md` 문서 업데이트.
- [x] **install.sh에서 Docker 없이 --with-server 지원** — `check_docker()`를 boolean 반환으로 변경. `check_native_prereqs()` 추가 (Go, Node, pnpm, PostgreSQL 검증). `setup_server_native()` 추가 (네이티브 빌드 및 실행, PID 파일 관리). `run_with_server()`가 Docker/네이티브 자동 분기. `run_stop()`이 PID 파일 기반 네이티브 프로세스 종료 지원.
- [x] **Bootstrap 토큰 자동 생성** — `MULTICA_BOOTSTRAP_EMAIL` 환경변수 설정 시 서버 시작 시 자동으로 admin 유저 + 1년 만료 PAT 토큰 생성. `server/cmd/server/bootstrap.go` 신규 파일. 토큰은 최초 1회만 생성되고 stderr에 출력. `.env.example`에 문서화.
- [x] **multica setup에서 기존 토큰으로 로그인 스킵** — `server/cmd/multica/cmd_setup.go`에 `tokenIsValid()` 함수 추가. `runSetupCloud()`/`runSetupSelfHost()`에서 유효한 토큰이 있으면 브라우저 로그인 건너뛰고 workspace만 자동 설정.
- [x] **프로젝트별 Working Folder 설정** — `project` 테이블에 `working_folder TEXT` 컬럼 추가 (migration 064). sqlc 쿼리 및 Go 핸들러에 working_folder CRUD 반영. ClaimTask 응답에 project의 working_folder 전달. 데몬의 runTask에 3단계 WorkDir 우선순위 (WorkingFolder > PriorWorkDir > Prepare). `execenv.ReuseCustomFolder()` 함수 추가 (폴더 존재/쓰기 검증, context 파일만 주입, GC 자동 제외). 기존 CLAUDE.md 보존 로직 (`RuntimeConfigFilename()` 헬퍼). 프론트엔드 타입/생성 모달/상세 사이드바에 Working Folder 필드 추가.

## Backlog

- [ ] **Working Folder 동시 접근 정책** — 같은 프로젝트에 여러 에이전트가 동시에 할당되면 같은 폴더에서 작업하게 됨. Git worktree 분리 또는 동시 실행 제한 정책 검토 필요.
- [ ] **Working Folder에 생성되는 .agent_context/ 정리** — 에이전트 작업 후 `.agent_context/` 폴더가 사용자 프로젝트에 남음. 자동 정리 정책 또는 `.gitignore` 자동 추가 검토.
- [ ] **CLI 기반 로그인 (브라우저 없이)** — 터미널에서 이메일 + 인증코드를 직접 입력하여 토큰 발급. 헤드리스 환경 지원.
