# Issue Status Transitions

## Status 종류

| Status | 설명 | 아이콘 |
|--------|------|--------|
| `backlog` | 파킹랏 — 아직 작업 대상 아님 | 빈 원 |
| `todo` | 작업 대상이지만 아직 시작 전 | 25% 원 |
| `in_progress` | 에이전트 또는 사람이 작업 중 | 50% 원 |
| `in_review` | 작업 완료, 리뷰 대기 | 75% 원 (초록) |
| `done` | 완료 | 채워진 원 (초록) |
| `blocked` | 진행 불가 — 외부 의존 또는 문제 | 경고 아이콘 |
| `cancelled` | 취소됨 | 취소선 원 |

## 자동 전환 (서버)

서버가 태스크 라이프사이클에 따라 이슈 상태를 자동으로 전환한다. 에이전트가 CLI로 먼저 상태를 변경한 경우 덮어쓰지 않는다.

```
태스크 시작 (StartTask)
  현재 상태가 todo 또는 backlog → in_progress

태스크 성공 완료 (CompleteTask)  
  현재 상태가 in_progress → in_review

태스크 실패 (FailTask, 재시도 없는 경우)
  현재 상태가 in_progress이고 활성 태스크 없음 → todo
```

### 전환 매트릭스

| 현재 상태 | 이벤트 | 자동 전환 | 비고 |
|-----------|--------|-----------|------|
| `backlog` | 태스크 시작 | -> `in_progress` | |
| `todo` | 태스크 시작 | -> `in_progress` | |
| `in_progress` | 태스크 성공 완료 | -> `in_review` | |
| `in_progress` | 태스크 실패 (재시도 없음, 활성 태스크 없음) | -> `todo` | 기존 로직 |
| `in_review` | 태스크 시작 (코멘트 트리거) | 변경 없음 | 리뷰 중 추가 작업은 상태 유지 |
| `done` | - | 변경 없음 | |
| `blocked` | 태스크 시작 | 변경 없음 | 에이전트가 의도적으로 설정한 상태 |
| `cancelled` | - | 변경 없음 | 취소 시 활성 태스크도 함께 취소됨 |

## 에이전트 오버라이드

에이전트는 `multica issue status <id> <status>` CLI로 언제든 상태를 변경할 수 있다. 서버 자동 전환은 "에이전트가 아직 변경하지 않은 경우"의 안전망이다.

### 에이전트가 오버라이드하는 대표 케이스

- **바로 `done`**: 테스트 수정 등 리뷰가 불필요한 간단 작업
- **`blocked`**: 외부 의존성, 권한 부족, 모호한 요구사항 등으로 진행 불가
- **`in_progress` 유지**: 태스크 완료 후에도 후속 작업이 필요한 경우 (자동 `in_review` 전에 에이전트가 먼저 변경)

## 사용자/멤버 전환

사용자는 UI 또는 API를 통해 어떤 상태로든 자유롭게 전환할 수 있다. 워크플로우가 강제되지 않는다.

### 사용자 전환 시 부수 효과

| 전환 | 부수 효과 |
|------|-----------|
| `backlog` -> 활성 상태 (`todo`/`in_progress` 등) | 에이전트 assignee면 태스크 자동 생성 |
| 어떤 상태 -> `cancelled` | 활성 태스크 전부 취소 |
| assignee 변경 | 기존 태스크 취소 + 새 에이전트에 태스크 생성 |

## 태스크 생성 트리거

| 트리거 | 조건 |
|--------|------|
| 이슈 생성 시 에이전트 assignee | 상태가 `backlog`이 아닌 경우 |
| 이슈 상태 변경 (`backlog` -> 활성) | 에이전트 assignee + 런타임 연결 |
| 이슈에 멤버 코멘트 | 에이전트 assignee + 대기 중 태스크 없음 |
| assignee 변경 | 새 assignee가 에이전트 + 런타임 연결 |

## Autopilot과 상태

- **`create_issue` 모드**: autopilot이 이슈를 생성하면 일반 이슈와 동일한 상태 전환 규칙 적용
- **`run_only` 모드**: 이슈가 없으므로 상태 전환 대상 없음. autopilot run이 완료/실패로 관리됨
- **Autopilot run 완료 조건**: 이슈가 `done` 또는 `in_review`에 도달하면 해당 run을 완료 처리

## 구현 위치

| 로직 | 파일 | 함수 |
|------|------|------|
| 자동 `in_progress` | `server/internal/service/task.go` | `StartTask` |
| 자동 `in_review` | `server/internal/service/task.go` | `CompleteTask` |
| 실패 시 `todo` 복귀 | `server/internal/service/task.go` | `HandleFailedTasks` |
| `cancelled` 시 태스크 취소 | `server/internal/handler/issue.go` | `UpdateIssue` |
| 태스크 생성 트리거 | `server/internal/handler/issue.go` | `shouldEnqueueAgentTask` |
| AGENTS.md 워크플로우 | `server/internal/daemon/execenv/runtime_config.go` | `BuildAgentsMD` |
