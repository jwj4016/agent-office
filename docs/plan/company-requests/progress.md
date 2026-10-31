# 회사 요청 접수 — 진행 기록

계획: [plan.md](plan.md)

## 2026-10-30 — 계획 작성

```text
날짜: 2026-10-30
현재 단계: 계획(구현 전)
완료한 체크리스트: 없음
```

- 계기: 빌라노트 MVP 실행(`run-f724riwj3z2cxry233na`, 16단계)이 성공했지만, 코드 결과가 앱 저장소의 작업 브랜치에만 남았다. 마지막 브랜치는 `agent-office/f724riwj/web-frontend-g1-a1`(`11097fd`, 파이프라인·백엔드·프론트 포함)이고, `master`는 첫 커밋이다. 다음 실행은 `master`에서 시작하므로 1차 결과를 이어받지 못한다.
- 사용자 결정: 결과를 사람이 직접 꺼내지 않고, 회사에 요청하면 접수 담당이 처리한다. 접수 담당은 회사 전체와 서비스마다 모두 둔다.
- 실제 데이터 상태(읽기 전용 확인): 서비스 VillaLens·RealstateLens·빌라노트, 부서 0개, 업무 0건. 빌라노트는 작업 폴더 없음, 자동 모드, 허용 연결 없음.

```text
미검증 항목과 막힌 이유: 없음(구현 전)
다음에 할 구체적인 작업: 0단계 — feat/service-operations 병합 여부를 사용자에게 확인하고, 새 브랜치에서 기준 시험·DB 백업.
```

## 2026-10-30 — 0단계(병합 제외)와 1단계 완료: 코드 결과 전달

```text
날짜: 2026-10-30
현재 단계: 1단계 완료
완료한 체크리스트: 0단계 3/4(병합 남음), 1단계 전체 6개
```

- 0단계: PR #3(`feat/service-operations` → main)을 만들었고 CI(Windows·macOS·Ubuntu) 통과. 병합은 자동 권한 검사에서 막혀 사용자가 직접 하기로 함. `feat/company-requests`는 `feat/service-operations` 끝 커밋(`35ba6c3`)에서 시작. 기준 시험: Go 전체 통과, 프론트 164건·타입 검사 통과. 실제 DB 백업 `~/Library/Application Support/AgentOffice-backup-20261030-231859.db`(스키마 16, 무결성 검사 ok). 개발 앱(`wails dev`)이 실제 데이터로 켜져 있었고 진행 중인 AI 작업은 없었음.

**변경한 코드**

- 엔진 `internal/engine/deliver.go`: `FinalCode`(성공한 실행의 마지막 코드 단계 커밋, 합친 커밋, 바뀐 파일 수, 서비스 기준 코드 포함 여부, 전달 기록), `DeliverCode`(방식 `base`·`folder`·`branch`·`advance`). 여러 마지막 단계는 임시 작업 트리에서 `agent-office/<실행>/final`로 합치고, 충돌이면 브랜치를 지우고 멈춤. 전달은 엔진 안에서 하나씩.
- Git 도우미: `FastForward`, `CurrentBranch`, `BranchCommit`, `CreateBranch`, `DeleteBranch`, `Clone`(원격 연결 삭제).
- 바인딩 `FinalCode`, `DeliverCode`. `frontend/wailsjs` 다시 생성.
- 화면 `views/CodeDelivery.tsx`: 성공한 실행 화면의 ‘코드 결과’. 앱 저장소면 ‘폴더로 꺼내기’(빈 폴더, 작업 폴더로 쓰기 기본 켬)·‘기준 코드만 갱신’, 사용자 저장소면 ‘결과 브랜치 만들기’·‘현재 브랜치에도 반영’. 기준 코드 포함 여부와 전달 기록.
- 결정 2건 `docs/decisions.md`. 계획 9.1절의 `code_deliveries` 테이블은 실행 사건으로 바꿈.

**실행한 검증과 결과**

| 명령 | 결과 |
|---|---|
| `go vet`, `go test`(`build/` 제외 전체) | 통과. 새 시험 `TestDeliverCodeToFolder`(꺼내기·작업 폴더 등록·원격 없음·빈 폴더만·다음 실행이 꺼낸 코드에서 시작), `TestDeliverMergesLastCodeSteps`(갈라진 두 단계 합치기, 충돌이면 멈추고 브랜치·기준 코드 그대로), `TestDeliverToUserRepository`(결과 브랜치, 커밋 안 된 변경이 있으면 반영 안 함, 깨끗하면 빨리 감기, 같은 브랜치 재사용). 바인딩 시험에 다른 서비스 실행 거부 |
| `npm test`, `npm run typecheck`, `npm run build` | 통과(34개 파일, 167건). 새 시험 `CodeDelivery.test.tsx` 3건 |
| 실제 데이터 사본으로 `FinalCode`(읽기만) | 빌라노트 실행 `run-f724riwj…`의 최종 코드 = ‘웹 클라이언트 구현’ `11097fd`, 파일 90개. 실제 저장소 `master`·작업 트리 그대로 |

```text
미검증 항목과 막힌 이유: 실제 앱 화면에서 빌라노트 코드를 꺼내는 것은 하지 않음(사용자 폴더에 쓰고 서비스 작업 폴더를 바꾸는 일이라 사용자가 고르거나 6단계에서 함). Windows의 Git 복제·경로(CI 결과 전).
다음에 할 구체적인 작업: PR #3 병합 뒤 main을 이 브랜치에 합치기. 2단계 — 접수 담당·요청·처리 항목 테이블(마이그레이션 0017, 적용 전 실제 DB 백업), 설정 화면, 서비스 ‘요청’ 탭.
```

## 2026-10-31 — 2~6단계 완료: 접수 담당, 요청, 회사 조치, 회사 접수, 실제 AI 확인

```text
날짜: 2026-10-31
현재 단계: 6단계 완료(0단계의 PR #3 병합만 남음)
완료한 체크리스트: 2~6단계 전체, 7장 최종 인수 체크리스트(시험용 공급자 기준, 대표 예시는 실제 AI)
```

**설계 변경(계획 2차 개정):** 요청의 AI 호출은 `work_ai_jobs`를 다시 만들지 않고 요청 전용 테이블에 기록. 마이그레이션 0017은 새 테이블과 열 하나만 더함. 0017 적용 전 실제 DB 백업 `~/Library/Application Support/AgentOffice-backup-20261030-233419-pre0017.db`. 켜져 있던 `wails dev`가 재빌드하며 실제 DB에 0017을 적용했음(스키마 17, 요청 0건).

**변경한 코드**

- 저장소: `migrations/0017_requests.sql`, `repo_requests.go`(접수 담당, 요청 제출·목록, 실행기용 대기열·점유·시도·해제·실패, 판단 저장, 사람의 답·닫기·취소·재시도, 업무 등록·거절, 조치 시작·끝·확인, 재시작 복구, 내 할 일, 회사 접수 사용량), `WorkLineage`의 요청 원인, `SumAIAttemptUsage`에 요청 시도 합산.
- 실행기: `internal/work/intake.go`(접수 담당 검사 → 점유 → 호출 → 검사 → 저장, 자동 재시도), `intake_prompt.go`(회사·서비스 판단 입력, 40KB 상한, 결과 스키마, 목록에 없는 ID 거부).
- 엔진: 코드를 바꾸지 않은 코드 단계는 ‘코드 결과’에서 뺌, 전달 기록에 요청 항목 ID.
- 앱: `app_requests.go`(접수 담당·요청·조치 바인딩), 내 할 일·알림에 요청, 내보내기·결과 저장을 ‘창 → 쓰기’로 나눔.
- 화면: `Requests.tsx`(서비스 ‘요청’ 탭, 사이드바 ‘회사에 요청’, 내 할 일의 ‘요청’, 조치 승인), `IntakeDesk.tsx`(회사 조직·부서·담당자의 접수 담당), 업무 ‘시작한 이유’에 요청.
- 문서: `docs/company-requests.ko.md`, README, `docs/acceptance.md`, `docs/decisions.md` 3건.

**실행한 검증과 결과**

| 명령 | 결과 |
|---|---|
| `go vet`, `go test`(`build/` 제외 전체) | 통과. 새 시험: `intake_test.go` 7건, `TestUpgradeToRequestsKeepsWork`, `TestBindingsRequestActions`, `TestBindingsCompanyRequestDeliversCode`(대표 예시 전체: 회사 요청 → 넘김 → deliver_code 승인 → 폴더·작업 폴더 → 다음 실행) |
| `npm test`, `npm run typecheck`, `npm run build` | 통과(34개 파일, 173건). 새 시험 `Requests.test.tsx` 6건 |
| 실제 DB 사본에 0017 적용 | 스키마 17, 서비스 3개 유지, 외래 키 위반 0 |
| 실제 Claude(sonnet)로 대표 예시, 실제 데이터 사본(`TestRealVillaNoteRequests`) | 성공(55초, 호출 4회, $0.38). 회사 접수 → 빌라노트, deliver_code 제안·실행, 검토 지적에서 업무 5건, QA 요약 답변. 실제 데이터는 바뀌지 않음(요청 0건, 앱 저장소 `master` 그대로) |

**구현 중 찾아 고친 결함:** 실행기가 오래된 대기열로 이미 점유된 요청을 ‘대기’로 바꿀 수 있던 경쟁 조건, 앱 시작 때 복구가 Wails 런타임 없는 시험에서 화면 알림을 보내 프로세스를 끝내던 문제, 아무것도 바꾸지 않은 코드 단계가 ‘코드 결과’로 보이던 문제, 판단 저장 사건의 상태 값.

```text
미검증 항목과 막힌 이유: 실제 앱 화면(확인하려던 때 사용자의 `wails dev`가 재빌드 뒤 앱을 다시 띄우지 않았고, 사용자 터미널의 프로세스라 다시 시작하지 않음 — jsdom 구성요소 시험만). 요청에서 만든 업무를 실제 AI로 실행하는 것(비용). Windows·Linux(CI 전). 판단 품질은 한 번의 표본.
다음에 할 구체적인 작업: PR #3 병합 → 이 브랜치에 main 합치기 → 실제 앱에서 접수 담당 지정과 ‘빌라노트 코드 내 폴더로 보내 줘’ 요청(사장님이 승인하며 폴더 선택).
```

## 2026-10-31 — 3차 개정: 접수 담당 자동 배치

```text
날짜: 2026-10-31
현재 단계: 3차 개정 완료
완료한 체크리스트: 3차 개정 전체(자동 배치, 세 가지 지정 방법, 지침 다시 쓰기, 폴더 제안)
```

- PR #3을 main에 병합했고(사용자 지시), main을 이 브랜치에 합침.
- 마이그레이션 `0018_desk_staffing.sql`. 적용 전 백업 `~/Library/Application Support/AgentOffice-backup-20261031-150712-pre0018.db`. 구현 중에는 개발 앱을 꺼서 실제 대기 요청이 덜 만든 코드로 처리되지 않게 함.
- 코드: `internal/work/staffing.go`(배치·지침 다시 쓰기), 실행기의 `staffFor`, `repo_staffing.go`(공통 역할, 기록, 선호 연결, 담당자 만들기), `ParkRequest`, 바인딩 `StaffIntakeDesk`·`RewriteIntakeDesk`·`CreateIntakeAssignee`, 화면 `IntakeDesk.tsx`(세 가지 방법), 요청 카드의 배치 표시와 폴더 미리 채움.
- 검증: Go 전체, 프론트 35개 파일 177건, 타입 검사, 빌드 통과. 실제 Claude로 사본의 대기 요청 처리(`TestRealDeskStaffing`, 34초, $0.12).
- 실제 AI 결과를 보고 고친 것: 새로 배치된 접수 담당에게 방금 배치됐다는 사실을 알리고, 대표가 읽는 글에 내부 ID를 쓰지 않게 지시.

```text
미검증 항목과 막힌 이유: 판단 품질은 표본 1. Windows·Linux(CI 결과 전).
다음에 할 구체적인 작업: 요청을 결과가 나올 때까지 따라가기(완료 보고) 검토.
```

## 2026-10-31 — 4차 개정: 서비스 시작 때 조직 구성

- 마이그레이션 `0019_org_proposals.sql`. 적용 전 백업 `~/Library/Application Support/AgentOffice-backup-20261031-220154-pre0019.db`. 구현 중 개발 앱을 꺼 둠.
- 코드: `domain/org.go`(조직 계획·검사), `storage/repo_orgplan.go`(적용, 업무를 새 부서로), `storage/repo_orgproposals.go`, `work/organize.go`(조직 정비 제안·적용), 설계안 스키마·지시·검사·적용, 바인딩 `LatestOrgProposal`·`ProposeOrganization`·`ApplyOrganization`·`DiscardOrgProposal`, 화면 `Organize.tsx`(부서·담당자 탭의 ‘조직 정비’, 설계 검토의 ‘부서와 접수 담당’).
- 검증: Go 전체, 프론트 36개 파일 179건, 타입 검사, 빌드 통과. 실제 Claude `TestRealOrganize`(사본, 67초).
- 고친 것: 조직 정비 미리보기의 연결 선택이 화면을 다시 읽을 때 처음 값으로 돌아가던 문제.

```text
미검증 항목과 막힌 이유: 실제 빌라노트에 조직 정비를 적용하는 것은 사장님이 미리보기를 보고 할 일로 남김. 예산이 없어 자동 시작은 검토로 넘어감. Windows·Linux(CI 결과 전).
다음에 할 구체적인 작업: 계획서 ‘다음 단계: 협업 요청의 끝까지 처리’.
```

## 2026-10-31 — 5차 개정: 협업 요청의 끝까지 처리

- `progress.md` 맨 위에 붙어 있던 채팅 문장을 지우고 커밋함(사용자 지시).
- 마이그레이션 `0020_request_followthrough.sql`. 적용 전 백업 `~/Library/Application Support/AgentOffice-backup-20261031-221932-pre0020.db`. 구현 중 개발 앱을 꺼 둠.
- 코드: 요청의 공동 목표·순서(`ApplyIntakeDecision`, `AcceptRequestWork`, `ensureRequestGoalTx`), 업무 상태가 닫히거나 다시 열릴 때 요청 갱신, 완료 보고 실행기 `internal/work/report.go`와 `repo_reports.go`, 자동 시작의 앞 업무 대기(`WorkWaitingFor`, `NoteAutoStart`), 바인딩 `MarkReportRead`, 화면(요청 카드의 진행·공동 목표·앞 업무·자동 시작 이유·완료 보고, 업무 상세의 앞 업무).
- 검증: Go 전체, 프론트 36개 파일 180건, 타입 검사, 빌드 통과. 실제 Claude `TestRealFollowThrough`(사본).
- 실제 AI로 찾아 고친 것: 보고 대기 목록이 요청의 업무 항목을 읽지 않아 보고 지시에 업무가 빠짐(시험에서 발견), AI가 보고 세부 내용을 객체로 보내 파싱이 실패함.

```text
미검증 항목과 막힌 이유: 실제 업무를 AI로 끝까지 실행해 그 결과로 쓴 보고(사본에서는 완료 표시만 함, 실행 비용). Windows·Linux(CI 결과 전).
다음에 할 구체적인 작업: 사용자가 실제 빌라노트에 조직 정비를 적용하고 예산을 정한 뒤, 요청 업무가 순서대로 자동 실행되고 보고가 붙는지 확인.
```
