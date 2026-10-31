# 인수 테스트 증거

명세 14장의 T01~T24 구현·시험 증거와 플랫폼별 결과를 기록한다.

## 실제 공급자 시험 기록

| 날짜 | 공급자 | 환경 | 결과 |
|---|---|---|---|
| 2026-10-03 | Codex App Server | codex-cli 0.159.2, macOS arm64, `TestCodexReal` | 성공: started→message→usage→completed(succeeded), 7.1s. 취소 미시험 |
| 2026-10-07 | Codex·Claude 연결 시험 | codex-cli 0.160.0 / Agent SDK 0.3.288, 실제 호출 시험 화면 | 둘 다 4단계 통과 |
| 2026-10-07 | M2 게이트 흐름 | 실제 Claude·Codex, `wails dev`, 한글·공백 작업 폴더 | 기획→승인→개발→리뷰→검증 완료 |
| 2026-10-07 | 실제 취소 | `TestCodexRealCancel`, `TestClaudeRealCancel` | Codex 통과 / Claude 남은 프로세스 결함 수정 후 통과 |
| 2026-10-08 | M4 게이트 흐름 | Windows 11, 실제 Claude(Agent SDK, claude.ai 로그인)·Codex(codex-cli 0.161.0), `TestRealM4Gate` | 분기 skipped→합류, 회의, 병렬 worktree→통합 실제 빌드·테스트, 리뷰 수정 1회차, QA·전달까지 완료(1476s). 도구 허가 14건 사람 처리 |
| 2026-10-03 | Claude Agent SDK bridge | SDK 0.3.288, Node 26.10.0, `TestClaudeReal` | 성공: started→message→usage($0.0485)→completed(succeeded), 5.4s. 취소 미시험 |
| 2026-10-26 | 서비스 운영 대표 시나리오 | 실제 Claude(이 PC 로그인, sonnet), macOS arm64, `TestRealServiceOperations` | 성공(2분 28초, AI 시도 11회, 보고 비용 $0.66). 아래 ‘서비스 운영’ 표의 실제 AI 결과 참고. 이 시험에서 Claude 사용량의 입력 토큰에 캐시 토큰이 빠지는 결함을 찾아 고침 |
| 2026-10-31 | 회사 요청 대표 시나리오 | 실제 Claude(이 PC 로그인, sonnet), macOS arm64, 실제 데이터 사본(저장소 경로를 사본으로 바꿈), `TestRealVillaNoteRequests` | 성공(55초, 접수 호출 4회, 보고 비용 $0.38, 알 수 없는 비용 0회). 아래 ‘회사 요청’ 표 참고 |

상태: ⬜ 미착수 · 🟡 구현(자동 테스트만) · ✅ 검증 완료 · ⚠️ 미검증(환경 없음)

| ID | 상황 | 상태 | 증거 (테스트·명령·결과) |
|---|---|---|---|
| T01 | 사용자가 역할·흐름 생성 | 🟡 | 테스트 provider: `TestServiceDevFlowEndToEnd`, M1 화면 시험. 실제 AI(2026-10-07, macOS): 사용자 구성 흐름이 실제 Claude·Codex로 의존 순서대로 완료(M2 게이트). Windows·Linux 미시험 |
| T02 | “게임 출시, 리뷰는 내가” 요청 | 🟡 | 테스트 provider: `TestDesignReviewModeNewService`(사람 업무→local-owner 배정을 코드로 검증). 실제 AI(2026-10-08, macOS, Codex): 설계안이 기존 역할 4개(기획·개발·QA·리뷰어)를 모두 재사용(중복 역할 없음), '코드 리뷰' 단계가 사람(local-owner)에게 배정, 검토 후 적용 → 흐름 편집기에서 버전 1 확정(M3 게이트). 배포 자료 부족은 '부족한 항목'으로 표시되어 실행은 보류. Windows·Linux 미시험 |
| T03 | auto 모드, 허용 범위 충족 | 🟡 | `TestDesignAutoModeStartsAndWaitsForPeople`(범위 충족 시 적용·실행 후 사람 업무에서 대기), `TestDesignAutoModeRefusedOutsideScope`(허용 밖 연결·예산 없음·부족 항목이면 거부). 화면: `Design.test.tsx`(범위 밖이면 자동 시작 비활성). 실제 AI로는 미시험 |
| T04 | 연결 없는 AI 배정 | 🟡 | 검증: `TestValidateAssignments`, `TestConfirmRejectsInvalidAndAllowsUnconnected`. 바인딩: `TestBindingsStartBlockedWithoutConnection` (버전 확정 가능, 실행은 설명과 함께 차단). 화면: 담당자 목록에 '확인 필요' 표시 |
| T05 | 사람이 리뷰 수정 요청 | 🟡 | `TestReviewChangesReworkOnlyAffectedSteps` (백엔드·통합·리뷰·QA·전달만 재실행, 프론트엔드 유지, 이전 결과 stale 보존, 의견 전달). Git 작업 공간: `TestCodeReworkContinuesFromPreviousCommit`, M4 게이트(이전 커밋에서 이어서 수정, 통합 재실행·실제 빌드) |
| T06 | 기획 반려 | 🟡 | `TestPlanRejectionKeepsIndependentResults` (기획 의존 범위만 재실행, 독립 분석 유지, 이전 승인 stale) |
| T07 | 게임·부동산 서비스 실행 | 🟡 | `TestInboxAndDashboardAcrossServices`, `TestBindingsTwoServicesEndToEnd`, 실제 앱 화면 시험: 두 서비스 동시 실행, 내 할 일·대시보드 서비스별 표시, 교차 접근 거절, 이벤트 섞임 0건. 실제 AI 세션 분리는 M2 |
| T08 | 서비스 전환·한 서비스 일시 정지 | 🟡 | 엔진: `TestPauseOneRunOthersContinue`. 화면: 서비스 전환은 화면 선택만 바꾸며 실행 유지(실제 앱에서 두 서비스 실행 중 전환 확인), 마지막 서비스 재시작 후 복원 |
| T09 | 조건 분기 한 경로 미선택 | 🟡 | `TestConditionSkipsUntakenBranch`, `TestConditionTakesBranch`, `TestConditionPathErrorFails`. M4 게이트 `TestM4GateBranchParallelRevision`(건너뛴 법률 검토 뒤 합류 진행). 편집기: 분기 규칙·합류 편집(`NodeForm.test.tsx`), 편집기 분기 모양 검증 `TestEditorBranchScaffoldIsValid` |
| T10 | 실패한 필수 선행 업무 | 🟡 | `TestFailureBlocksOnlyDependents` (후속 차단, 독립 작업 계속, 재시도), `TestRunFailsWhenBlocked` |
| T11 | 이전 승인·이벤트가 늦게 도착 | 🟡 | `TestLateResultFromSupersededAttemptIsIgnored` (늦은 결과는 provider.late로만 기록, 새 시도는 이전 프로세스 종료 후 시작), 이전 승인 stale (`TestPlanRejectionKeepsIndependentResults`) |
| T12 | 도구 승인 거절 | 🟡 | 공급자·엔진 시험(`TestToolApprovalDecline` 등). 실제 Claude의 Bash 승인 요청을 내 할 일에서 처리(허용 경로 확인). 실제 공급자 거절 경로는 가짜 시험만 |
| T13 | 실행 중 초안·공통 역할 편집 | 🟡 | 버전 스냅샷 고정: `TestVersionSnapshotIsFrozen` (초안·역할 지침·담당자 변경 후 v1 불변, v2에 반영). 실행 중 시나리오는 엔진 항목에서 |
| T14 | 두 개발 AI의 병렬 변경 | 🟡 | `TestParallelCodeStepsIntegrateAndBuild`(별도 worktree·브랜치, 통합 worktree에서 병합 후 실제 `go build ./...` 검증, 원본 작업 트리 불변), 충돌: `TestIntegrationConflictMustBeResolved`, 재작업: `TestCodeReworkContinuesFromPreviousCommit`, 사용자 저장소 보존: `TestUserRepositoryIsLeftAlone`. 실제 AI(2026-10-08, Windows): Codex 백엔드·Claude 프론트엔드가 별도 worktree에서 동시 작업, Claude 통합이 병합 후 `go build`·`go vet`·`go test ./...` 통과(`TestRealM4Gate`) |
| T15 | 모델은 완료 주장, 테스트 실패 | 🟡 | `TestClaimedDoneWithoutOutputFails`, `TestFailingVerificationCommandFails`, 사람 리뷰도 동일 기준: `TestReviewPassMeetsCompletionCriteria` |
| T16 | 앱 강제 종료·재시작 | 🟡 | `TestRecoverAfterEngineStopped`, 실제 앱 `kill -9` 후 재시작(테스트 provider). 실제 공급자 실행 중 강제 종료는 미시험 |
| T17 | 프로세스 취소 | 🟡 | 가짜: `TestCodexCancelKillsStuckProcessTree`, `TestClaudeCancelLeavesNoChildren`. 실제(2026-10-07, macOS): `TestCodexRealCancel`, `TestClaudeRealCancel` — 취소 후 프로세스 그룹에 남은 프로세스 없음. 프로세스 묶음(M6, `internal/proc`): 부모가 먼저 끝난 손자 프로세스까지 정리 `TestKillReachesOrphanedGrandchild`(macOS·Linux·Windows CI), Windows Job Object 닫힘 시 정리 `TestCloseEndsTheJob`·콘솔 창 숨김 `TestHideSetsNoWindow`(windows-latest, run 38575688557). 설치 패키지 진단의 실행 취소(4개 패키지). 실제 공급자 취소는 Windows에서 미시험 |
| T18 | 질문이 서로 순환 또는 무응답 | 🟡 | 무응답: `TestAIQuestionEscalatesWhenUnanswered`(제한 시간 후 사용자에게 넘김, 사람 답변 후 계속). 순환: `TestAIQuestionLoopIsCapped`(같은 두 담당자 사이 한도 초과 질문은 사용자에게), 보조 세션은 질문 불가(고정 답변). 회의: 라운드 한도 `TestMeetingRunsRoundsThenAIDecides`, 합의 시 조기 종료 `TestMeetingStopsWhenAllAgree`. 실제 공급자 미시험 |
| T19 | 예산·수정 한도 도달 | 🟡 | 수정 한도: `TestRevisionLimit`. 예산: `TestBudgetHoldsNewAIWork`(새 AI 업무 보류·사람 대기·내 할 일 표시·상향 시 재개), `TestUsageUnknownCost`(미보고 비용 별도 집계). 실제 공급자 미시험 |
| T20 | 키 저장소 없음·내보내기 | 🟡 | 키 저장소 없을 때 메모리 fallback: `secrets_test.go` TestOpenFallsBackToMemory. 내보내기: `TestExportExcludesKeys`(끝난 실행을 내보내 저장된 키·키 형태 문자열 가림, secret://참조·실행 파일 경로·공급자 세션 없음, 연결은 id·이름·종류만, 다른 서비스 행 없음, 없는 결과 파일은 목록에만), `TestReadInsideStaysInTheDataFolder`(데이터 폴더 밖·심볼릭 링크 거부), `TestExportProjectBinding`(저장 대화상자·.zip 붙임·임시 파일 남지 않음·취소), 화면 `ExportButton.test.tsx`. 실제 앱 저장은 M5 게이트 |
| T21 | 한글·공백 경로, 설치 패키지 | 🟡 | DB 경로: `TestMigrationsApplyOnceAndPersistAcrossReopen`. 실제 AI 작업 폴더 '게이트 작업 폴더'에서 Codex 작성·Claude 검증·검증 명령 정상(M2 게이트). 설치 패키지(M6, CI run 38575688891): dmg arm64·x64를 '설치 시험/응용 프로그램', NSIS 설치 파일을 `C:\에이전트 오피스 시험\Agent Office`에 설치하고 deb는 /usr/lib에 설치. 이후 설치된 실행 파일의 `--self-check`가 '에이전트 오피스 진단 *' 데이터 폴더·'작업 폴더 (진단)' 작업 폴더에서 저장·다시 열기, 포함된 Claude bridge 실행(SDK 0.3.288, Claude 2.1.288), 테스트 AI 흐름 9업무·결과 8개 해시 확인, 취소, 재시작 복원을 모두 통과. Windows·Linux는 제거까지 확인(`selfcheck_test.go`). 실제 AI 공급자로 설치 패키지에서 업무를 실행한 것은 아님 |
| T22 | 도트 모션 줄이기·키보드 | 🟡 | `views/Office.test.tsx`: 화살표·Home/End로 담당자 선택과 상태 문장 낭독(aria-live), Enter로 상세와 '다시 시도'·'실행 보기', '목록' 보기에서 같은 상세·'내 할 일에서 처리', 모션 줄이기(앱 설정·OS) 시 애니메이션 타이머 없음(설정 불러오기 전에도 정지). 흐름 편집: 끌기 대신 '선행 업무 추가' 선택(`NodeForm.test.tsx`, 순환 후보 제외). 상세 패널은 열릴 때만 초점 이동·Esc로 닫기(`ui.test.tsx`). 좁은 창은 사무실 대신 목록. 실제 앱(2026-10-14, macOS): 키보드 선택·상세, 목록 보기 선택 기억, 모션 줄이기 시 캔버스 고정 확인 |
| T23 | 악성 프로젝트·산출물 ID 참조 | 🟡 | 저장: `TestCrossProjectReferencesRejected`, `TestCheckScope`. 엔진: `TestEngineRejectsCrossProjectIDs`. 바인딩: `TestBindingsTwoServicesEndToEnd`, 실제 앱 콘솔에서 교차 GetRun·ReadArtifact 거절 확인 |
| T24 | 두 앱 인스턴스·이중 완료 클릭 | 🟡 | 이중 클릭: `TestStepApprovalDecidedOnce`, `TestDuplicateApprovalClicks`. 두 인스턴스: 데이터 폴더 잠금 `TestSecondOpenIsLocked`, `TestSecondAppInstanceRefused`(3 OS CI), 엔진 1개 `TestSecondEngineRefusedWhileFirstRuns`, macOS 빌드 앱 두 번 실행 시 두 번째 즉시 종료(2026-10-03). Windows·Linux 설치 앱으로 두 번 실행하는 화면 시험은 미시험 |

## 플랫폼별 설치 시험 (M6)

`.github/workflows/package.yml`: 패키지 생성 → 한글·공백 경로에 설치 → 설치된 실행 파일의 `--self-check` → 제거. 진단은 bridge가 패키지에 포함된 것(`claude-runner`)인지까지 확인한다.

| 패키지 | 크기 | 설치 위치 | 진단 | 제거 | 증거 |
|---|---|---|---|---|---|
| macOS arm64 dmg | 133MB | `$RUNNER_TEMP/설치 시험/응용 프로그램` | 전 항목 ok (Codex 미설치 warn) | — (앱 폴더 삭제) | run 38575688891, macos-latest |
| macOS x64 dmg | 140MB | 같은 방식 | 전 항목 ok (Codex warn) | — | run 38575688891, macos-15-intel |
| Windows x64 설치 exe | 95MB | `C:\에이전트 오피스 시험\Agent Office` | 전 항목 ok (Codex warn) | 앱 파일 삭제 확인 | run 38575688891 windows-latest에서 빌드·시험, run 38581308212 Linux 교차 빌드 → windows-latest 시험 |
| Ubuntu 24.04 deb | 99MB | `/usr/lib/agent-office` | ok, 키 저장소 warn(CI에 Secret Service 없음), Codex warn | 실행 파일 삭제 확인 | run 38575688891, ubuntu-24.04 |
| macOS arm64 (개발 Mac) | — | `scratchpad/설치 시험/응용 프로그램` | 전 항목 ok | — | 2026-10-14 수동, macOS 26.4 |

서명: macOS는 ad-hoc 서명(`codesign --verify --strict` 통과), Windows는 서명하지 않음. 실제 화면으로 첫 실행 경고(Gatekeeper·SmartScreen)를 확인하지는 않았다.

## 성능 측정 (M6, 명세 §11)

시나리오: 서비스 2개(담당자 10명씩, 총 20명), 50단계 흐름 1개와 서비스 개발 흐름 1개. 테스트 AI는 업무당 20초가 걸리고, 앱 전체 동시 실행 한도 2 때문에 '실행 중'에는 AI 업무 2개가 계속 작업한다. 첫 화면은 50단계 서비스의 사무실이다. 측정은 `scripts/perf.sh`(macOS·Linux)와 `scripts/perf.ps1`(Windows)로 한다(`--perf-seed`, `AGENT_OFFICE_PERF`).

- 시작: 프로세스 시작부터 데이터가 있는 첫 화면이 그려질 때까지. 실행 3회 값이다.
- CPU: 앱과 그 WebView 프로세스의 누적 CPU 시간 증가분을 측정 시간으로 나눈 값. 코어 1개가 100%다.
- 메모리: RSS 합계(Windows는 작업 집합 합계). 공유 메모리가 프로세스마다 중복 계산된다.
- 화면 지연: 페이지 안 100ms 타이머의 지연. 10초마다 p95·최대를 기록한다.

| 환경 | 프로세스 | 시작(ms) | 유휴 CPU | 유휴 메모리 | 실행 중 CPU | 실행 중 메모리(최대) | 화면 지연 p95 / 최대 (실행 중) |
|---|---|---|---|---|---|---|---|
| 개발 Mac, M1 Pro·32GB, macOS 26.4 (dev 빌드, 2026-10-14) | 4 | 534 / 532 / 551 | 5.9% | 242MB | 7.3% | 261MB | 5 / 6ms |
| CI macOS arm64 (macos-latest VM) | 4 | 4699 / 1853 / 1913 | 0.4%¹ | 245MB | 1.5% | 268MB | 8 / 115ms |
| CI macOS x64 (macos-15-intel VM) | 4 | 5563 / 3764 / 1590 | 1.5% | 134MB | 5.6% | 154MB | 8 / 27ms |
| CI Windows x64 (windows-latest VM) | 7 | 8434 / 488 / 479 | 0.4% | 349MB | 2.3% | 377MB | 11 / 16ms |
| CI Ubuntu 24.04 (ubuntu-24.04 VM, xvfb) | 3 | 1590 / 313 / 307 | 3.3% | 460MB | 5.0% | 476MB | 5 / 7ms |

CI 수치는 run 38581308212에서 나왔다. CI는 가상 머신이고, 첫 실행에는 디스크 캐시가 비어 있어 느리다. 사용자의 PC 성능을 보장하는 값이 아니다. Windows·Linux 실제 PC에서는 아직 측정하지 않았다.

¹ CI macOS arm64의 유휴 구간은 화면 지연 p95가 900ms였다. 페이지는 hidden이 아니었지만 앱이 앞에 있지 않아 macOS가 타이머를 늦춘 것으로 보이며(App Nap 등), 사무실 애니메이션이 거의 돌지 않은 값이다. 앞에 띄운 창 기준은 개발 Mac 행을 본다.

측정하면서 고친 것: 사무실 캔버스가 애니메이션 프레임(170ms)마다 `canvas.width`를 다시 지정해 버퍼를 다시 할당하고 있었다. 크기가 바뀔 때만 지정하도록 고쳤다(`Office.test.tsx` 'redraws animation frames without resizing the canvas', 수정 전 실패 확인). 개발 Mac의 유휴 CPU는 첫 측정 14.2%에서 수정 후 5.9~7.5%가 됐다. 다만 첫 측정은 창 상태를 기록하기 전이라 엄밀한 비교는 아니다. 남은 유휴 CPU는 대부분 화면에 보이는 사무실 애니메이션(약 6fps)이며, 모션 줄이기를 켜거나 다른 탭으로 가면 멈춘다.


## 서비스 운영(사건 기반 협업) 인수 증거

[서비스 운영 계획서](plan/service-operations/plan.md) 7장 최종 인수 체크리스트의 증거다. 자동 시험은 정해 둔 응답을 쓰는 시험용 공급자로 돌렸으므로 AI가 협업 필요성을 판단하는 품질은 따로 검증하지 않았다(🟡). 실제 앱 확인은 macOS에서 `wails dev`와 임시 데이터 폴더로 했고, 앱에 들어 있는 시험용 AI는 정해진 형식으로 답하지 않아 화면에서는 실패·재시도 흐름만 확인했다(2026-10-26). Windows·Linux는 CI의 Go·프론트 시험만 통과했다.

| 항목 | 상태 | 증거 |
|---|---|---|
| 고정 그래프 없이 기획안 공유에서 개발·재무의 별도 업무 | 🟡 | `TestDepartmentsProposeFromASharedPlan`, `TestBindingsDepartmentsProposeWork` |
| 다른 업무에서는 개발팀만 참여하고 재무팀은 대응 불필요 | 🟡 | `TestDepartmentsProposeFromASharedPlan`(두 번째 자료) |
| 사용자 제안 요청과 정기 제안을 함께 사용 | 🟡 | `TestChecksAreInteractiveAndSubscriptionsBackground`, `TestRegularChecksRunOncePerSlot`, `TestBindingsDepartmentSchedule`. 실제 앱에서 정기 점검 한 회차 실행 확인 |
| 명시적 도움 요청·구독 참여·자료 직접 활용 | 🟡 | `TestBindingsCollaborationCompletesGoal`, `TestWorkEventsReachSubscribedDepartments`, `TestSharedDraftIsUsedByOtherDepartments` |
| 직접 요청은 지정 부서에만, 자료 접근·예산 제한 유지 | 🟡 | `TestDirectCheckIgnoresSubscriptions`, `TestDocumentScopeLimitsWhoSeesIt`, `TestReviewExistingDocuments`, `TestBudgetHoldsJudgment` |
| 완료 전 초안 공유와 두 팀의 병렬 작업 | 🟡 | `TestBindingsShareDraftWithAnotherDepartment`, `TestNextRunContinuesFromSharedDraft` |
| 원본이 바뀌어도 사용한 입력 판 유지 | 🟡 | `TestShareAndReviseDocument`, `TestLinkingAnotherVersionReplacesTheLink` |
| 새 판·철회는 그 자료를 쓴 업무에만 영향 확인 | 🟡 | `TestShareAndReviseDocument`, `TestWithdrawAndSearch` |
| 초안 참조와 업무적 승인 구분 | 🟡 | `TestConfirmVersionWithoutChangingIt`, `TestSharedDraftIsUsedByOtherDepartments` |
| 같은 판 확정(해시·번호 유지, 사건 한 번, 새 내용은 초안) | 🟡 | `TestConfirmVersionWithoutChangingIt` |
| 확정 사건은 알림만, 직접 고른 부서만 판단 | 🟡 | `TestConfirmVersionWithoutChangingIt`, `TestWorkEventsReachSubscribedDepartments` |
| 필수·참고용 완료 규칙, 순환 대기 차단 | 🟡 | `TestCollaborationGatesTheGoal`, `TestRequiredCycleRefused` |
| 수행 업무 없는 필수 요청도 완료를 막고 해제·대체 기록 | 🟡 | `TestRequiredRequestWithoutWorkBlocks`, `TestReplaceRequiredRequest` |
| 재시작·중복 전달·판단 후 종료·등록 후 종료에도 업무 하나 | 🟡 | `TestDrainHandlesEachDeliveryOnce`, `TestRestartFinishesInterruptedDelivery`, `TestJudgmentSurvivesRestarts`, `TestAcceptingAProposalRegistersWorkOnce` |
| 같은 회차 재시도와 질문 답변만 다음 회차, 두 종류의 재시작 복구 | 🟡 | `TestFailedCallsAreRetriedAndPaidFor`, `TestQuestionThenAnswerThenDecide`, `TestSecondQuestionGoesToAPerson`, `TestInterruptedCallIsRecovered`, `TestReadyJudgmentIsNotCalledAgain` |
| 반응할 일이 없으면 업무를 만들지 않음 | 🟡 | `TestDepartmentsProposeFromASharedPlan`(대응 불필요) |
| 연쇄 깊이·연쇄 업무 수·사건당 부서 수 한도에서 보류 | 🟡 | `TestDeepAutomaticChainsAreHeld`, `TestChainHoldLimits`, `TestJudgeFanoutIsLimited` |
| 판단·계획의 모든 시도 사용량 합산, 중복 없음, 알 수 없는 비용 | 🟡 | `TestPlanProposalIsAppliedAndPaidFor`, `TestFailedCallsAreRetriedAndPaidFor`, `TestUsageUnknownCost`. 실제 앱 ‘현황’에서 알 수 없는 비용 호출 수 표시 확인 |
| 예산 보류 중 새 판단·계획·재시도·자동 실행 시작 안 함 | 🟡 | `TestBudgetHoldStopsTheCall`, `TestBudgetHoldsJudgment`, `TestAutoStartChecksSendWorkToReview` |
| 검토 모드·자동 모드가 기존 권한·예산 준수 | 🟡 | `TestReviewModeWaits`, `TestAutoStartChecksSendWorkToReview`, `TestAutoModeRunsAllowedWorkAndSharesResults` |
| 사람이 부서를 대신한 결정과 AI 결정의 구분 | 🟡 | `TestAICollabDecisions`, `TestWorkLineageAndDepartmentLoads`(원인의 결정자) |
| 자동 모드 AI의 참고용 협업 결정, 필수는 사람만, 되돌리기 | 🟡 | `TestAICollabDecisions`, `TestBindingsAutoModeStartsAllowedWork`, `TestFailedAICollabDecisionGoesToThePerson` |
| 백그라운드가 쌓여도 사용자 요청은 바로, 동시 2건 이하 | 🟡 | `TestLanesDoNotWaitForEachOther`, `TestChecksAreInteractiveAndSubscriptionsBackground` |
| 새 부서에 과거 사건을 재전달하지 않음 | 🟡 | `TestNewDepartmentJoinsLaterEvents` |
| 서비스 두 개의 자료·사건·업무가 섞이지 않음 | 🟡 | `TestWorkScopedToService`, `TestDepartmentsBelongToTheirService`, `TestCrossProjectReferencesRejected`, `TestBindingsTwoServicesEndToEnd` |
| 업그레이드 후 기존 역할·워크플로·실행·산출물 사용 | 🟡 | `TestUpgradeFromV2KeepsData`, `TestUpgradeToDepartmentsKeepsServices`, `TestUpgradeToAutomationKeepsCollabHistory`, `TestUpgradeToSchedulesKeepsDepartments` |
| (9단계) 어떤 사건 때문에 이 부서가 이 업무를 시작했는지 설명하고 따라가기 | 🟡 | `TestWorkLineageAndDepartmentLoads`, 프론트 `Work.test.tsx`·`Operations.test.tsx`·`Inbox.test.tsx`. 실제 앱에서 협업 업무의 원인·요청한 업무·이어진 업무 이동 확인 |

### 실제 AI로 돌린 대표 시나리오 (2026-10-26)

`AGENT_OFFICE_REAL_OPERATIONS=sonnet go test . -run TestRealServiceOperations -v -timeout 60m`(실제 Claude, 이 PC의 로그인, macOS). 계획서 1장 대표 예시와 7장 항목을 앱 바인딩으로 그대로 따라간다. 기계적 규칙(누구에게 전달되는지, 무엇이 저장·차단되는지)은 실패로 검사하고, AI의 판단은 기록만 한다.

| 단계 | 결과 |
|---|---|
| 마케팅 기획안 초안 공유(랜딩 페이지·가입 폼·쿠폰, 광고 예산 3,000만 원) | 개발팀 ‘캠페인 랜딩 페이지·가입 폼·쿠폰 발급 구현’, 재무팀 ‘캠페인 총예산 및 쿠폰 비용 추정 검토’ 제안. 구독 없는 법무팀은 전달도 호출도 없음 |
| 제안 등록 | 각 한 번만 등록(두 번째 등록 거절), 두 업무의 원인이 ‘캠페인 기획안’·‘캠페인 기획’으로 나옴 |
| 개발만 필요한 알림 문구 개선안 공유 | 개발팀은 딥링크 수정 업무 제안, 재무팀은 ‘새 지출 없음’으로 대응 불필요 |
| 기획안 2판 공개 | 1판을 연결한 개발 업무에만 영향 확인, 기획 업무 자신은 제외. 두 팀 모두 ‘이미 진행 중인 업무 범위’라며 대응 불필요(중복 업무 없음) |
| 재무팀 필수 협업 | 미충족이면 기획 완료 거절, 수행 업무 완료 뒤 완료됨 |
| 자동 모드 참고용 협업(배너 화면 변경 정리) | 개발팀 대표 AI가 수락(`decidedBy=ai`) → AI 계획 → 승인 → 실제 실행 → 결과 1건 초안 공유 → ‘결과 검토’ 대기 |
| 재무팀 정기 점검(회차 실행) | 백그라운드 판단, ‘이미 진행 중인 업무가 있어 새 일 없음’ |
| 운영 현황 | 부서별 진행·결과 확인·제안 수가 위 결과와 일치, 전달 실패·보류 없음 |

판단 품질 메모: 이번 한 번의 실행에서 제안·불필요 판단은 모두 자료 내용과 맞았고 이유가 구체적이었다. 표본이 한 번뿐이어서 품질을 일반화하지 않는다.

## 회사 요청(접수 담당) 인수 증거

[회사 요청 계획서](plan/company-requests/plan.md) 7장 최종 인수 체크리스트의 증거다. 자동 시험은 정해 둔 응답을 쓰는 시험용 공급자로 돌렸다(🟡). 실제 AI 확인은 실제 데이터의 사본으로 했고, 사장님의 실제 데이터(요청·접수 담당·작업 폴더·앱 저장소)는 바뀌지 않았음을 확인했다. 실제 앱 화면 확인은 하지 못했다: 확인하려던 때 켜져 있던 `wails dev`가 재빌드 뒤 앱을 다시 띄우지 않았다. 화면은 jsdom 구성요소 시험으로만 확인했다. Windows·Linux는 CI 결과 전이다.

| 항목 | 상태 | 증거 |
|---|---|---|
| 회사 요청이 알맞은 서비스로만 넘어감 | 🟡 | `TestCompanyDeskRoutesToServices`(두 서비스, 중복 넘김 제거), `TestArchivedServicesAreNotRoutedTo`. 실제 AI: “빌라노트 코드 내 폴더로 보내 줘”가 빌라노트 하나로 넘어감 |
| 서비스를 고르면 회사 접수 AI를 부르지 않음 | 🟡 | `TestCompanyDeskRoutesToServices`(호출 기록), 화면 `Requests.test.tsx` |
| 답변·업무·조치·되묻기·처리 불가 구분 | 🟡 | `TestServiceDeskDecides`, `TestAutoWorkAndActions`. 실제 AI: 조치(deliver_code), 업무 5건, QA 요약 답변 |
| 되묻기는 2회차까지, 이후 사람이 처리 | 🟡 | `TestServiceDeskDecides`(답 → 2회차), 저장소의 2회차 질문은 `manual`(`ApplyIntakeDecision`), 화면의 ‘직접 처리하고 닫기’ |
| 검토 모드는 사람이 등록, 자동 모드는 바로 등록 | 🟡 | `TestServiceDeskDecides`, `TestAutoWorkAndActions`(‘자동 등록’, 원인 요청 기록) |
| 조치는 사람 승인 뒤 한 번만 실행 | 🟡 | `TestAutoWorkAndActions`, `TestBindingsRequestActions`(저장 창을 닫으면 실행 안 함, 두 번째 실행 거부, 다른 서비스 거부) |
| 코드 전달이 사용자 작업 트리·현재 브랜치를 건드리지 않음 | 🟡 | `TestDeliverToUserRepository` |
| 비어 있지 않은 폴더로는 꺼내지 않음 | 🟡 | `TestDeliverCodeToFolder` |
| 갈라진 코드 단계 합치기, 충돌이면 멈춤 | 🟡 | `TestDeliverMergesLastCodeSteps` |
| 전달 뒤 다음 실행이 전달된 코드에서 시작 | 🟡 | `TestDeliverCodeToFolder`, `TestBindingsCompanyRequestDeliversCode`(이전 버전 재실행은 앱 저장소의 갱신된 기준 코드, 새 버전은 꺼낸 폴더) |
| 비용 합산과 보류 | 🟡 | `TestServiceDeskDecides`(서비스 예산 4회), `TestRequestsWaitForDeskAndBudget`(서비스 예산 보류, 회사 월 예산 보류). 실제 AI: 회사 접수 1회 $0.052 |
| 재시작·중복 | 🟡 | `TestRequestRecoveryAndCancel`(접수 중 중단 → 다시 대기열), `TestAutoWorkAndActions`(조치 중 중단 → ‘확인 필요’, 저절로 다시 실행 안 함), 판단 저장은 한 트랜잭션, 넘김 하나에 하위 요청 하나(`UNIQUE(parent_item_id)`) |
| 요청에서 만든 업무의 ‘시작한 이유’ | 🟡 | `TestServiceDeskDecides`(`WorkLineage` via request) |
| 기존 기능이 업그레이드 뒤에도 동작 | 🟡 | `TestUpgradeToRequestsKeepsWork`, 실제 DB 사본에 0017 적용(서비스 3개 유지, 외래 키 위반 0), Go 전체·프론트 전체 시험 |

### 실제 AI로 돌린 대표 시나리오 (2026-10-31)

실제 데이터 사본, sonnet. 회사 접수 담당과 빌라노트 접수 담당(새 AI 담당자)을 두고 요청 3건을 보냈다.

1. 회사에 “빌라노트 코드 내 폴더로 보내 줘” → 회사 접수가 빌라노트로 넘김 → 빌라노트 접수가 MVP 실행(`run-f724riwj…`)의 `deliver_code`를 이유(“코드가 아직 서비스 기준 코드에 들어 있지 않습니다… 합성 자료 단계 구현”)와 함께 제안 → 사본 안의 임시 폴더로 승인 → 꺼내기와 작업 폴더 등록 성공.
2. 빌라노트에 “QA·전문가 검토에서 나온 문제 고쳐 줘” → 업무 5건(설계 문서 보완, 평가 기준서 보완(전세가율·역전세·사실 확인형 위험 신호), 백엔드 수정(중복 거래·금칙어), 프론트엔드 수정(추정 범위·고지 문구), 재검증). 서비스가 자동 모드여서 바로 등록됨. 접수 담당이 “최신 코드는 이미 서비스 기준 코드에 들어 있어 코드 반영 조치는 필요 없다”고 판단함. 재검증 업무가 앞 업무 뒤에 와야 한다는 순서는 업무 사이에 표현할 수 없어 이유 문장에만 남음.
3. 빌라노트에 “QA 결과를 한 문단으로 요약해 줘” → QA 보고서의 요약으로 답함(‘조건부 통과’, 시험 수, 고치지 않은 설계 문서).

### 접수 담당 자동 배치 (회사 요청 3차 개정, 2026-10-31)

| 항목 | 상태 | 증거 |
|---|---|---|
| 접수 담당 없이 온 요청에 AI가 새 접수 담당을 배치하고 이어서 판단 | 🟡 | `TestMissingDeskIsStaffedByAI`(서비스·회사, 공통 역할 하나, 앱이 정한 연결·모델, 요청 카드의 배치 표시, 배치 비용의 서비스 예산 합산, 지침 다시 쓰기) |
| 자동 배치 실패는 요청당 2번까지, 이후 사람을 기다림 | 🟡 | `TestRequestsWaitForDeskAndBudget`, `TestBindingsRequestActions` |
| 새로 만들기·기존 인원 지정·AI에게 맡기기 | 🟡 | `TestBindingsDeskAssignees`, 화면 `IntakeDesk.test.tsx` |
| 요청에 적은 위치를 폴더로 제안·미리 채움(홈 안만) | 🟡 | `TestSuggestedFolder`, 화면 `Requests.test.tsx` |
| 실제 AI | ✅(표본 1) | 실제 데이터 사본, 사장님이 보낸 대기 요청으로 `TestRealDeskStaffing`: Claude가 ‘빌라노트 접수 담당’을 배치(지침: 서비스 단계, 범위 밖 요청 처리, 담당자별 배정 기준, 대표에게 올릴 결정, 보고 방식), 새 접수 담당이 deliver_code와 폴더 `~/dev/workspace/villanote`를 제안. 34초, 배치 호출 $0.12. 승인은 하지 않음 |

### 서비스 시작 때 조직 구성 (회사 요청 4차 개정, 2026-10-31)

| 항목 | 상태 | 증거 |
|---|---|---|
| 설계안에 부서·접수 담당이 들어가고 적용 때 함께 만들어짐(검토 모드는 자동 시작 꺼짐, 자동 모드는 켜짐) | 🟡 | `TestDesignReviewModeNewService`, `TestDesignAutoModeStartsAndWaitsForPeople`, 설계 검사 `bad_org` |
| 기존 서비스의 조직 정비 제안·적용, 연결 허용, 멈춘 업무를 부서로 옮겨 자동 시작 대기열에 넣음 | 🟡 | `TestOrganizeExistingService`, `TestOrganizeRefusesUnknownMembers`, `TestBindingsOrganization`, 화면 `Organize.test.tsx` |
| 실제 AI | ✅(표본 1) | 실제 데이터 사본 `TestRealOrganize`(67초): 빌라노트 조직 제안 4개 부서(기획·분석실, 데이터·플랫폼팀, 제품팀, 품질·보안 검증실)와 받아 볼 사건이 서비스에 맞게 나옴. 적용 → 멈췄던 업무 2건이 부서로 들어가 자동 시작 대기열에 올랐고, 예산이 없어 ‘자동 시작 검토’로 넘어감(미리보기의 경고대로). 새 목표 설계(동네 카페 웹페이지)는 부서 2개와 접수 담당을 함께 내고 적용됨 |

### 협업 요청의 끝까지 처리 (회사 요청 5차 개정, 2026-10-31)

| 항목 | 상태 | 증거 |
|---|---|---|
| 요청이 업무가 닫힐 때까지 ‘진행 중’, 완료 보고로 끝남, 보고는 확인할 때까지 할 일 | 🟡 | `TestServiceDeskDecides`, `TestAutoWorkAndActions`(모두 취소되면 보고 없이 끝남), `TestUpgradeFollowsRequestsToTheEnd`, 화면 `Requests.test.tsx` |
| 공동 목표와 업무 순서, 자동 시작이 앞 업무를 기다림, 순서와 다르게 등록해도 관계 유지 | 🟡 | `TestGoalAndOrderedWork` |
| 보고 세부 내용이 글·목록·객체 어느 것으로 와도 읽을 수 있게 씀 | 🟡 | `TestRenderReport` |
| 실제 AI | ✅(표본 1) | 실제 데이터 사본 `TestRealFollowThrough`: 조직 정비 뒤 “역전세 지표를 추가해 줘…검증까지” 요청 → 공동 목표(책임 데이터·개발부)와 업무 4건(기준 → 계산 → 화면 → 검증, 앞 업무 연결). 자동 시작은 첫 업무만 진행(예산이 없어 검토로)하고 나머지는 앞 업무를 기다림. 처음에는 AI가 세부 내용을 객체로 보내 보고 호출 3회가 실패($0.16씩)하고 앱이 기록으로 보고를 대신 썼음 → 형식에 관대하게 고친 뒤 다시 돌려 AI 보고가 쓰임($0.15). 사본에서 결과물 없이 완료로 표시했더니 보고가 “결과물이 없어 이뤄졌다고 판단할 근거가 없다”고 정직하게 씀 |
