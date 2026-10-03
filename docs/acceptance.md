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
| T01 | 사용자가 역할·흐름 생성 | ⬜ | |
| T02 | “게임 출시, 리뷰는 내가” 요청 | ⬜ | |
| T03 | auto 모드, 허용 범위 충족 | ⬜ | |
| T04 | 연결 없는 AI 배정 | 🟡 | 검증 단계: `TestValidateAssignments`, `TestConfirmRejectsInvalidAndAllowsUnconnected` (버전 가능, canRun=false). 실행 차단·UI 설명은 엔진·UI 항목에서 |
| T05 | 사람이 리뷰 수정 요청 | ⬜ | |
| T06 | 기획 반려 | ⬜ | |
| T07 | 게임·부동산 서비스 실행 | ⬜ | |
| T08 | 서비스 전환·한 서비스 일시 정지 | ⬜ | |
| T09 | 조건 분기 한 경로 미선택 | ⬜ | |
| T10 | 실패한 필수 선행 업무 | ⬜ | |
| T11 | 이전 승인·이벤트가 늦게 도착 | ⬜ | |
| T12 | 도구 승인 거절 | 🟡 | 공급자 수준: 가짜 Codex/Claude가 decline 수신 확인 (`TestCodexApprovalRoundTrip`, `TestClaudeApprovalRoundTrip`). 엔진 기록은 M2 |
| T13 | 실행 중 초안·공통 역할 편집 | 🟡 | 버전 스냅샷 고정: `TestVersionSnapshotIsFrozen` (초안·역할 지침·담당자 변경 후 v1 불변, v2에 반영). 실행 중 시나리오는 엔진 항목에서 |
| T14 | 두 개발 AI의 병렬 변경 | ⬜ | |
| T15 | 모델은 완료 주장, 테스트 실패 | 🟡 | 공급자 수준: 완료 신호 없으면 failed (`TestClaimedDoneWithoutCompletionIsFailure`, bridge `error result is a failure`). 산출물 검증은 M1 엔진 |
| T16 | 앱 강제 종료·재시작 | ⬜ | |
| T17 | 프로세스 취소 | 🟡 | macOS: 취소 시 손자 프로세스 종료 (`TestCodexCancelKillsStuckProcessTree`), bridge 무응답 시 강제 종료 (`TestClaudeCancel/cancel-ignored`). Windows 미시험 |
| T18 | 질문이 서로 순환 또는 무응답 | ⬜ | |
| T19 | 예산·수정 한도 도달 | ⬜ | |
| T20 | 키 저장소 없음·내보내기 | ⬜ | 키 저장소 없을 때 메모리 fallback: `secrets_test.go` TestOpenFallsBackToMemory. 내보내기는 M5 |
| T21 | 한글·공백 경로, 설치 패키지 | ⬜ | DB 경로 한글·공백: `storage_test.go` TestMigrationsApplyOnceAndPersistAcrossReopen, wails dev 수동 확인 (2026-10-03, macOS). 설치 패키지·AI 시작은 미시험 |
| T22 | 도트 모션 줄이기·키보드 | ⬜ | |
| T23 | 악성 프로젝트·산출물 ID 참조 | 🟡 | 저장 계층: 복합 FK로 교차 참조 거절 (`TestCrossProjectReferencesRejected`), `CheckScope` (`TestCheckScope`). bindings 적용은 M1 UI |
| T24 | 두 앱 인스턴스·이중 완료 클릭 | 🟡 | 저장 계층: 승인 결정 1회만 적용 (`TestStepApprovalDecidedOnce`). 엔진·두 인스턴스는 미시험 |
