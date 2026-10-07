# 인수 테스트 증거

명세 14장의 T01~T24 구현·시험 증거와 플랫폼별 결과를 기록한다.

## 실제 공급자 시험 기록

| 날짜 | 공급자 | 환경 | 결과 |
|---|---|---|---|
| 2026-10-03 | Codex App Server | codex-cli 0.159.2, macOS arm64, `TestCodexReal` | 성공: started→message→usage→completed(succeeded), 7.1s. 취소 미시험 |
| 2026-10-03 | Claude Agent SDK bridge | SDK 0.3.288, Node 26.10.0, `TestClaudeReal` | 성공: started→message→usage($0.0485)→completed(succeeded), 5.4s. 취소 미시험 |

상태: ⬜ 미착수 · 🟡 구현(자동 테스트만) · ✅ 검증 완료 · ⚠️ 미검증(환경 없음)

| ID | 상황 | 상태 | 증거 (테스트·명령·결과) |
|---|---|---|---|
| T01 | 사용자가 역할·흐름 생성 | 🟡 | 엔진: `TestServiceDevFlowEndToEnd`. 바인딩: `TestBindingsTwoServicesEndToEnd`. 실제 앱 화면 시험(2026-10-03, macOS, wails dev, 테스트 provider): 역할·담당자·흐름 편집·버전 확정·실행·승인·리뷰 완료. 실제 AI 연결은 M2 |
| T02 | “게임 출시, 리뷰는 내가” 요청 | ⬜ | |
| T03 | auto 모드, 허용 범위 충족 | ⬜ | |
| T04 | 연결 없는 AI 배정 | 🟡 | 검증: `TestValidateAssignments`, `TestConfirmRejectsInvalidAndAllowsUnconnected`. 바인딩: `TestBindingsStartBlockedWithoutConnection` (버전 확정 가능, 실행은 설명과 함께 차단). 화면: 담당자 목록에 '확인 필요' 표시 |
| T05 | 사람이 리뷰 수정 요청 | 🟡 | `TestReviewChangesReworkOnlyAffectedSteps` (백엔드·통합·리뷰·QA·전달만 재실행, 프론트엔드 유지, 이전 결과 stale 보존, 의견 전달) |
| T06 | 기획 반려 | 🟡 | `TestPlanRejectionKeepsIndependentResults` (기획 의존 범위만 재실행, 독립 분석 유지, 이전 승인 stale) |
| T07 | 게임·부동산 서비스 실행 | 🟡 | `TestInboxAndDashboardAcrossServices`, `TestBindingsTwoServicesEndToEnd`, 실제 앱 화면 시험: 두 서비스 동시 실행, 내 할 일·대시보드 서비스별 표시, 교차 접근 거절, 이벤트 섞임 0건. 실제 AI 세션 분리는 M2 |
| T08 | 서비스 전환·한 서비스 일시 정지 | 🟡 | 엔진: `TestPauseOneRunOthersContinue`. 화면: 서비스 전환은 화면 선택만 바꾸며 실행 유지(실제 앱에서 두 서비스 실행 중 전환 확인), 마지막 서비스 재시작 후 복원 |
| T09 | 조건 분기 한 경로 미선택 | 🟡 | `TestConditionSkipsUntakenBranch`, `TestConditionTakesBranch`, `TestConditionPathErrorFails` |
| T10 | 실패한 필수 선행 업무 | 🟡 | `TestFailureBlocksOnlyDependents` (후속 차단, 독립 작업 계속, 재시도), `TestRunFailsWhenBlocked` |
| T11 | 이전 승인·이벤트가 늦게 도착 | 🟡 | `TestLateResultFromSupersededAttemptIsIgnored` (늦은 결과는 provider.late로만 기록, 새 시도는 이전 프로세스 종료 후 시작), 이전 승인 stale (`TestPlanRejectionKeepsIndependentResults`) |
| T12 | 도구 승인 거절 | 🟡 | 공급자: `TestCodexApprovalRoundTrip`, `TestClaudeApprovalRoundTrip`. 엔진: `TestToolApprovalDecline` (거절 기록, 공급자에 decline 전달, 반복 클릭 불변) |
| T13 | 실행 중 초안·공통 역할 편집 | 🟡 | 버전 스냅샷 고정: `TestVersionSnapshotIsFrozen` (초안·역할 지침·담당자 변경 후 v1 불변, v2에 반영). 실행 중 시나리오는 엔진 항목에서 |
| T14 | 두 개발 AI의 병렬 변경 | ⬜ | |
| T15 | 모델은 완료 주장, 테스트 실패 | 🟡 | `TestClaimedDoneWithoutOutputFails`, `TestFailingVerificationCommandFails`, 사람 리뷰도 동일 기준: `TestReviewPassMeetsCompletionCriteria` |
| T16 | 앱 강제 종료·재시작 | 🟡 | `TestRecoverAfterEngineStopped`. 실제 앱(2026-10-07, macOS, wails dev, 테스트 provider auto-slow): 실행 중 `kill -9` → 재시작 시 interrupted, 자동 재실행 없음, 다시 시도로 재개. 실제 공급자 프로세스 미시험 |
| T17 | 프로세스 취소 | 🟡 | 하위 프로세스: `TestCodexCancelKillsStuckProcessTree`, `TestClaudeCancel`. 실제 앱: 실행 중 업무 취소 2초 내 반영, 되돌림 없음 안내(테스트 provider). 실제 Codex·Claude 프로세스 취소는 게이트에서 |
| T18 | 질문이 서로 순환 또는 무응답 | ⬜ | |
| T19 | 예산·수정 한도 도달 | 🟡 | 수정 한도: `TestRevisionLimit`. 예산: `TestBudgetHoldsNewAIWork`(새 AI 업무 보류·사람 대기·내 할 일 표시·상향 시 재개), `TestUsageUnknownCost`(미보고 비용 별도 집계). 실제 공급자 미시험 |
| T20 | 키 저장소 없음·내보내기 | ⬜ | 키 저장소 없을 때 메모리 fallback: `secrets_test.go` TestOpenFallsBackToMemory. 내보내기는 M5 |
| T21 | 한글·공백 경로, 설치 패키지 | ⬜ | DB 경로 한글·공백: `storage_test.go` TestMigrationsApplyOnceAndPersistAcrossReopen, wails dev 수동 확인 (2026-10-03, macOS). 설치 패키지·AI 시작은 미시험 |
| T22 | 도트 모션 줄이기·키보드 | ⬜ | |
| T23 | 악성 프로젝트·산출물 ID 참조 | 🟡 | 저장: `TestCrossProjectReferencesRejected`, `TestCheckScope`. 엔진: `TestEngineRejectsCrossProjectIDs`. 바인딩: `TestBindingsTwoServicesEndToEnd`, 실제 앱 콘솔에서 교차 GetRun·ReadArtifact 거절 확인 |
| T24 | 두 앱 인스턴스·이중 완료 클릭 | 🟡 | 이중 클릭: `TestStepApprovalDecidedOnce`, `TestDuplicateApprovalClicks`. 두 인스턴스: 데이터 폴더 잠금 `TestSecondOpenIsLocked`, `TestSecondAppInstanceRefused`, 엔진 1개 `TestSecondEngineRefusedWhileFirstRuns`, macOS 빌드 앱 두 번 실행 시 두 번째 즉시 종료(2026-10-03). Windows·Linux 미시험 |
