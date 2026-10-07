# 진행 상황

기준 명세: [독립 개발 계획서](agent-office-standalone-implementation-plan.ko.md)

**규칙**
- 항목 1개씩: 구현 → 테스트 → `[x]` 체크 → 아래 로그 갱신
- 카테고리(굵은 소제목) 완료 시 커밋, 마일스톤 게이트 통과 시 사용자 확인 후 다음 단계
- 실제 키·다른 OS가 없어 확인하지 못한 항목은 `⚠️ 미검증`으로 남기고 완료로 표시하지 않음

## 체크리스트


### M0 기술 실증
**환경**
- [x] Go·Node·Wails v2 CLI 설치, `wails doctor` 통과
- [x] `docs/` 진행 파일 4종 생성 (progress·decisions·environment·acceptance)

**프로젝트 골격**
- [x] `wails init` (React+TS+Vite), 디렉터리 구성 (`internal/*`, `runners/claude`, `tests/fixtures`)
- [x] frontend `test`·`typecheck`·`build` 스크립트 (Vitest)
- [x] binding 호출·이벤트 수신 왕복 확인

**저장·비밀정보**
- [x] SQLite(modernc) 연결, migration 번호 관리, 쓰기 직렬화
- [x] 저장 → 앱 재시작 → 복원 확인
- [x] go-keyring 키 저장, 불가 시 세션 메모리 fallback

**AI 최소 실행**
- [x] Provider 공통 인터페이스 (Capabilities·Start·Respond·Cancel·Resume·Events)
- [x] 테스트 provider (fixture 이벤트 재생)
- [x] Codex App Server stdio 최소 실행 — 실제 CLI 호출 성공 (2026-10-03)
- [x] Codex 취소 실제 확인 — `TestCodexRealCancel` 통과 (2026-10-07)
- [x] Claude SDK bridge (TS, JSONL) 최소 실행 — 실제 SDK 호출 성공 (2026-10-03)
- [x] Claude 취소 실제 확인 — `TestClaudeRealCancel` 통과 (2026-10-07, 남은 하위 프로세스 결함 수정 후)

**빌드**
- [x] macOS `wails build` 성공
- [x] GitHub Actions로 Windows·Linux 빌드 — `.github/workflows/build.yml`, run 37649324901에서 Windows·Linux·macOS 테스트·빌드 통과(2026-10-08)
- [x] 🚪 게이트: 실제/미검증 연결 구분, 저장·취소 확인, 버전·플랫폼 제약 기록 — 통과 (미검증 3건은 ⚠️로 유지)

### M1 기본 제품 (테스트 provider 기준)
**도메인·저장**
- [x] 엔터티 스키마 14종 + FK·유일성 제약
- [x] 상태 변경 + ExecutionEvent 동일 트랜잭션
- [x] 프로젝트 범위 검증 계층 (T23)

**흐름**
- [x] 흐름 JSON 스키마 + 검증 (ID·DAG·입출력 참조·reworkTargets·분기 규칙)
- [x] WorkflowVersion 스냅샷 고정 (T13)

**엔진**
- [x] 스케줄러: ready 판정, 동시 실행 한도(앱 2·프로젝트별), 입력 manifest 고정
- [x] 완료 기준 검증 — 모델 "완료" 주장만으로 성공 금지 (T15)
- [x] 사람 task·review·approval 대기/제출
- [x] 반려·재작업: generation 증가, 영향 범위만 재실행, 최대 3회 (T05·T06)
- [x] condition·join·skipped (T09), 실패 전파 (T10)
- [x] 늦은 이벤트·stale_request·이중 완료 방지 (T11·T24)
- [x] 실행별 pause·cancel (T08)

**UI**
- [x] 3단 레이아웃 + 서비스 대시보드 (생성·전환·보관)
- [x] 조직·역할·배정 화면, 합성 지침 미리보기
- [x] 흐름 편집 기본 (React Flow: 노드·연결·담당자)
- [x] 내 할 일·승인함, 결과·기록 화면
- [x] 🚪 게이트: 6.2 예시 흐름 끝까지 실행 + 두 서비스 분리 (T01·T07) — 통과 (테스트 provider 기준)

### M2 실제 AI 실행
- [x] Codex 연결 완성: 이벤트 정규화·승인/질문 응답·사용량 — 가짜 서버 시험, 실제 흐름 시험은 게이트에서
- [x] Claude bridge 완성: canUseTool·훅·질문·취소 — 가짜 bridge·단위 시험, 실제 흐름 시험은 게이트에서
- [x] OpenAI Responses·Claude Messages API 연결 (구조화 출력) — 가짜 HTTP 서버 시험, ⚠️ 실제 API 키 시험 전
- [x] 연결·설정 화면: 탐지·버전·인증·호출 성공 분리 표시 (T04)
- [x] 도구 승인 (T12), 사용량·비용·예산 (T19) — 테스트 provider 기준, 실제 공급자 확인은 게이트에서
- [x] 재시작 시 interrupted 처리·취소 확인 (T16·T17) — 실제 앱(테스트 provider) 확인, ⚠️ 실제 공급자 취소는 게이트에서
- [x] 🚪 게이트: 실제 기획 → 승인 → 개발 → 사람 리뷰 → 검증 — 통과 (2026-10-07, 실제 Claude·Codex)

### M3 AI 설계
- [x] 설계 요청 → JSON 초안 스키마·검증 (권한·키 자동 생성 금지)
- [x] 기존 역할 재사용/신규 추가 diff, 배정 생성
- [x] review·auto 모드, 사전 허용 범위 검증 (T03)
- [x] 목표 요청 화면 (새/기존 서비스, 변경 비교)
- [x] 🚪 게이트: "게임 출시, 리뷰는 내가" → local-owner 배정 (T02)

### M4 협업
- [ ] Message 7종 + AI 입력 컨텍스트 구성
- [ ] 협의 작업(회의), 순환·무응답 제한 (T18)
- [ ] Git worktree 분리·통합·테스트/빌드 (T14)
- [ ] 흐름 편집기: 조건·분기·수정 대상 편집 완성
- [ ] 🚪 게이트: 병렬 개발 통합 + 수정 회차 + 분기 합류 통과

### M5 사무실·템플릿
- [ ] Canvas 도트 사무실 + 실제 상태 매핑, 100ms 배치 반영
- [ ] 접근성: 모션 줄이기·키보드·목록 대체 (T22)
- [ ] 초기 템플릿 5종 (서비스 개발·게임 출시·법률·부동산·빈 흐름)
- [ ] 키 제외 내보내기 (T20)
- [ ] 🚪 게이트: 화면과 실제 상태 일치, 모든 템플릿 편집·실행

### M6 출시 검증
- [ ] 3 OS 설치 패키지 (exe·dmg·deb)
- [ ] 한글·공백 경로 (T21), 성능 측정 시나리오 기록
- [ ] 서명·공증 (자격증명 준비 시)
- [ ] 🚪 게이트: acceptance.md에 T01~T24 증거 + 알려진 제한 정리


## 작업 로그

### 2026-10-03
- 현재 단계: M0 / 환경
- 구현 완료: Homebrew로 Go·Node 설치, `go install`로 Wails CLI 설치, docs 진행 파일 4종 생성
- 이번 검증과 결과: `wails doctor` SUCCESS (macOS 26.4.1 arm64)
- 미검증·알려진 제한: Windows·Linux 환경 없음
- 다음에 실행할 구체적인 작업: `wails init` React+TS 골격 생성

- 현재 단계: M0 / 프로젝트 골격
- 구현 완료: Wails React+TS 골격, `internal/*` 패키지 자리, Vitest·typecheck 스크립트, `Ping` binding + `system:ping` 이벤트
- 이번 검증과 결과: `go test ./...` 통과, `npm test`·`typecheck`·`build` 통과, `wails dev` 실행 후 브라우저에서 binding 응답과 이벤트 수신 모두 확인
- 미검증·알려진 제한: Wails dev 브라우저 모드의 오버레이 스크립트 예외(앱 동작과 무관), macOS 링크 경고(아래 environment.md)
- 마지막 관련 코드/테스트: `app.go`, `app_test.go`, `frontend/src/App.test.tsx`
- 다음에 실행할 구체적인 작업: SQLite(modernc) 연결과 migration

- 현재 단계: M0 / 저장·비밀정보
- 구현 완료: `internal/storage` (modernc SQLite, WAL·foreign_keys, 번호형 embed migration, 단일 writer 트랜잭션, 허용 키 설정), `internal/secrets` (OS 키체인 probe 후 사용, 실패 시 세션 메모리, DB에는 `secret://` ref만), App bindings `SystemStatus`·`GetSettings`·`SetSetting`
- 이번 검증과 결과: `go test -race` 통과(재오픈 복원, 롤백, 동시 쓰기 50건, 한글·공백 경로), 실제 macOS Keychain 저장·조회·삭제 통과(`AGENT_OFFICE_KEYRING_IT=1`), `wails dev` 강제 종료 후 재시작 시 설정 복원 확인
- 미검증·알려진 제한: Windows Credential Manager·Linux Secret Service 미검증
- 마지막 관련 코드/테스트: `internal/storage/*_test.go`, `internal/secrets/secrets_test.go`, `app_test.go`
- 다음에 실행할 구체적인 작업: Provider 공통 인터페이스 정의

- 현재 단계: M0 / AI 최소 실행
- 구현 완료: `internal/providers` 공통 계약(정규화 이벤트, 단일 completed 보장), 테스트 provider + `tests/fixtures/providers/*.jsonl`, Codex App Server 어댑터(JSON-RPC stdio, 승인·질문·interrupt, 미지원 요청 거절), Claude bridge(`runners/claude`, Agent SDK `query`·`canUseTool`·abort, `settingSources: []` 격리) + Go `ClaudeBridge`, 하위 프로세스 그룹 종료
- 이번 검증과 결과: `go test -race -count=3 ./internal/providers/` 통과(가짜 Codex 서버·가짜 bridge를 실제 하위 프로세스로 실행, 취소 시 손자 프로세스까지 종료 확인), `runners/claude` `npm test` 5건 통과
- 미검증·알려진 제한: 실제 Codex·Claude 호출 미실행(사용자 계정 사용 승인 필요). Codex·Claude 둘 다 로컬 CLI 로그인 상태는 확인됨. Windows 프로세스 트리 종료는 taskkill 기반, 미시험. Claude bridge는 질문(question) 기능 미지원
- 마지막 관련 코드/테스트: `internal/providers/{provider,testprovider,codex,claude,process}.go`, `runners/claude/src/bridge.ts`
- 다음에 실행할 구체적인 작업: macOS `wails build`, GitHub Actions Windows·Linux 빌드

- 현재 단계: M0 / 빌드
- 구현 완료: `wails build` macOS arm64 앱, `.github/workflows/build.yml` (Ubuntu 24.04 `webkit2_41`·Windows·macOS 매트릭스: frontend·bridge·Go 테스트 후 Wails 빌드, 산출물 업로드)
- 이번 검증과 결과: macOS arm64 `.app` 12MB, 15초 빌드. 패키지 실행 시 한글·공백 데이터 경로에 DB 생성·migration 0001 적용 확인. workflow YAML 문법 확인
- 미검증·알려진 제한: Windows·Linux 빌드는 CI 미실행. macOS x64·서명·공증 미시험
- 마지막 관련 코드/테스트: `.github/workflows/build.yml`
- 다음에 실행할 구체적인 작업: M0 게이트 보고 — 실제 공급자 시험·CI 실행 여부 사용자 확인

- 현재 단계: M0 게이트
- 구현 완료: M0 전 항목 구현
- 이번 검증과 결과: 실제 Codex(codex-cli 0.159.2, 모델 gpt-6.1-sol) 1턴 성공 7초, 비용 미보고(null). 실제 Claude bridge(SDK 0.3.288, 모델 claude-opus-5-5) 1턴 성공 5초, 보고 비용 $0.0485
- 미검증·알려진 제한: 실제 공급자 취소, Windows·Linux CI 빌드(사용자 결정으로 push 보류)
- 다음에 실행할 구체적인 작업: 사용자 확인 후 M1 도메인·저장 — 엔터티 스키마 migration 0002

### 2026-10-03 (M1)
- 현재 단계: M1 / 도메인·저장 (브랜치 `feat/m1-core`)
- 구현 완료: migration 0002 엔터티 14종(프로젝트 소속 행은 `(id, project_id)` 복합 FK로 다른 프로젝트 참조를 DB가 거절), 기본 회사 `org-default`, 승인 결정 1회(`decision IS NULL` 조건 갱신, 업무 승인은 시도당 1건·도구 승인은 request_key별), `DB.Change`(상태+이벤트 한 트랜잭션, 커밋 후 순서대로 발행), `CheckScope`/`CheckScopeTx`, `NewID`
- 이번 검증과 결과: `go test -race ./internal/storage/` 통과 — 다른 프로젝트 참조 7종 거절, 중복·CHECK, 롤백 시 저장·발행 없음, 동시 40건 발행 순서 유지, 범위 검사(SQL 주입 문자열 포함)
- 미검증·알려진 제한: 기존 v1 DB에서 v2로 올리는 경로는 신규 DB 재오픈 시험으로만 확인
- 마지막 관련 코드/테스트: `internal/storage/{migrations/0002_core_entities.sql,events.go,scope.go}`, `*_test.go`
- 다음에 실행할 구체적인 작업: 흐름 JSON 스키마와 검증기 (`internal/domain`)

- 현재 단계: M1 / 흐름
- 구현 완료: `internal/domain` 흐름 타입·엄격 파서(unknown field 거절)·구조 검증(분기는 합류에서 만남, 중첩 분기·분기 밖 의존·모호한 필수 입력 거절)·담당자 검증(SevError=버전 불가, SevRun=실행 불가), 엔터티 타입·지침 합성. `storage` 역할(순환 거절)·프로젝트(보관)·담당자(사람은 local-owner)·연결·흐름 초안(revision 충돌 감지)·`ConfirmVersion`(흐름+담당자+합성 지침+정책 스냅샷). `internal/testenv` 시험용 프로젝트 구성
- 이번 검증과 결과: `go test -race ./internal/...` 통과. 흐름 오류 28종, T13 스냅샷 고정, T04 연결 없는 AI는 버전 가능·실행 불가
- 미검증·알려진 제한: 결과 JSON schema 자체의 검증은 엔진 완료 검증 항목에서 구현
- 마지막 관련 코드/테스트: `internal/domain/{workflow,validate,entities}.go`, `internal/storage/repo_*.go`, `repo_test.go`
- 다음에 실행할 구체적인 작업: 엔진 스케줄러 (`internal/engine`)

- 현재 단계: M1 / 엔진
- 구현 완료: `internal/engine` — 스케줄러(DB 기준, 앱 2·프로젝트 2 동시 실행, 입력 manifest 고정, 시도 생성 시 트랜잭션 안에서 재확인), AI 시도 실행(이벤트 저장, message_delta는 저장하지 않고 화면용 Ephemeral로 전달), 완료 검증(필수 결과·심볼릭 링크 거절·형식·JSON schema(외부 $ref 금지)·검증 명령은 최소 환경변수), 사람 task·review·approval(제출 검증 실패 시 단계 유지), 반려·재작업(generation·회차, superseded·stale, 수정 의견 전달, 기본 3회 한도), 조건·합류·skipped, 늦은 이벤트(provider.late)·stale_request·이중 클릭, 일시 정지·재개·취소·재시도, 시작 시 복구(interrupted). `superseded` 시도 상태 추가(0002 직접 수정, 실사용 DB 없음 확인)
- 이번 검증과 결과: `go test -race -count=8 ./internal/engine/` 통과 (테스트 21건)
- 미검증·알려진 제한: 예산 확인은 M2(T19). 재작업 중인 공급자 질문·도구 승인 대기 프로세스도 동시 실행 슬롯을 차지함. 사람 업무 제출은 텍스트만(파일 첨부는 UI 항목에서)
- 마지막 관련 코드/테스트: `internal/engine/*.go`, `*_test.go`
- 다음에 실행할 구체적인 작업: UI — 3단 레이아웃과 서비스 대시보드, App bindings

- 현재 단계: M1 / UI · 게이트
- 구현 완료: App bindings 40개(모두 프로젝트 범위 검사, 오류 한국어화, 검증 문제는 결과 구조체로 반환), 저장 이벤트 즉시 전달·스트리밍 글자 100ms 묶음 전달, 내장 시작 흐름(빈 흐름·서비스 개발 예시), 엔진 조회(내 할 일·대시보드·산출물 읽기+해시 확인). 화면: 3단 레이아웃(서비스 목록/본문/상세), 전체 서비스 대시보드(생성·전환·보관), 회사 조직(역할 계층), 담당자(AI·사람, 테스트 연결, 합성 지침 미리보기), 흐름 편집기(React Flow 노드·연결·삭제, 순환 즉시 거절, 노드 편집 패널, 키보드용 업무 목록, 저장·검증·버전 확정·실행), 실행 기록(진행·일시 정지·취소·재시도·결과 보기), 내 할 일(승인·보류·반려, 리뷰 통과·수정 요청, 작업 제출, 도구 승인, 질문 답변), 설정. 문구는 `i18n.ts`에 분리
- 이번 검증과 결과: `go test -race ./...` 통과(바인딩 수준 두 서비스 종단 시험 포함), 프론트엔드 테스트 16건·typecheck·build 통과. 실제 앱(`wails dev`, 새 데이터 폴더)에서 확인: 서비스 생성·역할·담당자·흐름 편집기 담당자 지정·버전 확정·실행 → 두 서비스 승인 → 게임은 백엔드만 수정 요청 후 재리뷰 통과(프론트엔드 v1 유지, 백엔드 v2) → 두 서비스 완료, 서비스 간 실행·산출물 접근 거절, 이벤트 섞임 0건, 앱 재시작 후 기록과 마지막 서비스 복원
- 미검증·알려진 제한: 반복 설정(역할 5개, 담당자 일부, 두 번째 서비스 구성)은 화면이 쓰는 같은 bindings를 브라우저 콘솔에서 호출해 진행. 실제 AI 연결로의 실행은 M2. 조건 분기 편집 UI는 M4. 노드 위치는 저장하지 않고 자동 배치
- 마지막 관련 코드/테스트: `app.go`, `bindings_test.go`, `frontend/src/**`, `internal/engine/inbox.go`, `internal/templates`
- 다음에 실행할 구체적인 작업: M1 게이트 보고 후 사용자 확인 → M2 Codex 연결 완성

- 현재 단계: M1 검토 결함 수정 (M2 착수 전)
- 구현 완료: 외부 검토 6건 수정 — ① 공급자 시작 실패 시 오류 변수 가림으로 인한 panic → 업무 실패로 처리, 스케줄·이벤트 처리에 panic 안전망 추가 ② 사람 리뷰 통과에 전체 완료 기준(모든 필수 결과·검증 명령) 적용, 리뷰 단계의 추가 결과 제출 지원(수정 요청은 보고서만) ③ 산출물·결과 읽기 경로를 완전 해석해 데이터 폴더 아래 심볼릭 링크 경로 거절 ④ 데이터 폴더 잠금 파일(`agent-office.db.lock`), DB당 엔진 1개(ClaimEngine), Wails 단일 인스턴스 옵션, 사용 중일 때 안내 화면 ⑤ 흐름 편집기 저장 실패 시 버전 확정 중단 ⑥ 승인 보류 후에도 승인·반려 버튼 유지
- 이번 검증과 결과: 수정 전 회귀 테스트로 ①③⑤⑥ 실패 재현(①은 nil pointer panic) 후 수정. `go test -race ./...` 통과, 엔진 `-count=2` 통과, 프론트엔드 20건·typecheck·build 통과, Windows·Linux 교차 vet 통과. 빌드한 앱을 같은 데이터 폴더로 두 번 실행 시 두 번째는 즉시 종료되고 1개만 남음
- 미검증·알려진 제한: Windows LockFileEx·단일 인스턴스는 미시험. 기존 `TestRecoverMarksInterrupted`는 첫 엔진이 살아 있는 상태에서 둘째 엔진을 띄워 잘못된 동작을 정상으로 고정하고 있었으므로 엔진 정지 후 복구하는 `TestRecoverAfterEngineStopped`로 교체
- 마지막 관련 코드/테스트: `internal/engine/{ai,human,verify,engine,schedule}.go`, `internal/engine/regress_test.go`, `verify_internal_test.go`, `internal/storage/{storage,lock_*}.go`, `frontend/src/views/{WorkflowEditor,Inbox}.tsx`
- 다음에 실행할 구체적인 작업: 사용자 확인 후 M2 Codex 연결 완성

### 2026-10-07 (M2)
- 현재 단계: M2 / 공급자 연결 (브랜치 `feat/m2-real-ai`)
- 구현 완료: 결과 폴더만 추가 쓰기 허용(Codex `sandboxPolicy.writableRoots`, Claude `additionalDirectories`), 검증 통과 결과를 `projects/<id>/artifacts/`로 읽기 전용 복사·해시 재확인(고정), Claude 인증 방식(API 키는 해당 프로세스에만 환경변수로 전달, 개인용 로그인 모드는 상속된 키 제거), Claude AskUserQuestion → 내 할 일 질문 연결, PreToolUse 쓰기 경로 훅(작업 폴더·결과 폴더 밖 파일 쓰기 거부, 심볼릭 링크 해석), 모델 API 공급자(Claude Messages: 구조화 출력 + `fallbacks: default`, 기본 모델 claude-opus-5-5 / OpenAI Responses: strict JSON schema, `store: false`, 모델 지정 필수), 단가 설정 시에만 비용 계산
- 이번 검증과 결과: `go test -race ./...` 통과(가짜 Codex의 writableRoots 확인, Claude 인증 환경 확인, 모델 API 가짜 HTTP 서버 5건), bridge `npm test` 9건 통과
- 미검증·알려진 제한: Codex는 이 PC의 CLI 로그인만 지원(API 키 방식은 키를 Codex 인증 파일에 저장하므로 제외). Bash 명령은 경로를 해석하지 않으므로 사람 승인에 의존. 실제 API 키 시험 없음
- 다음에 실행할 구체적인 작업: 연결·설정 화면(탐지·버전·인증·호출 시험 분리 표시, API 키 입력)

- 현재 단계: M2 / 연결·설정 화면
- 구현 완료: `internal/connect`(실행 파일 탐지: PATH + Homebrew·npm 등 고정 경로, Claude 실행 프로그램 탐지, 단계별 확인 실행 파일→버전→인증→실제 호출), 바인딩(연결 저장 시 확인 초기화, API 키 저장·삭제는 키체인에만, 무료 확인, "pong" 실제 호출 시험 성공 시에만 사용 가능 기록, 시험 중 도구·질문 요청 자동 거절), 설정 화면 AI 연결 관리(키 입력 후 다시 표시하지 않음, 실제 호출은 두 번 눌러 확인, 개인용 Claude 로그인 선택 시 안내 문구)
- 이번 검증과 결과: `go test -race ./...` 통과(가짜 CLI로 단계 확인, 가짜 Claude API로 키 저장→확인→사용 가능→설정 변경 시 초기화, 키가 목록·DB·오류에 나오지 않음), 프론트엔드 22건 통과
- 미검증·알려진 제한: 실제 Codex·Claude 연결 시험은 사용자 승인 후 게이트에서
- 다음에 실행할 구체적인 작업: 사용량·비용·예산 (T19)

- 현재 단계: M2 / 사용량·비용·예산
- 구현 완료: 시도별 마지막 보고 사용량을 서비스 단위로 합산(비용 미보고 시도는 별도 집계, 0원으로 표시하지 않음), 서비스 예산(토큰·보고된 비용) 초과 시 새 AI 업무만 보류하고 실행을 '사람 대기'로 표시·`budget.held` 기록·내 할 일에 '예산 한도' 항목, 예산 상향 시 즉시 재개. 서비스 개요에 사용량·예산 입력, 비용 미보고 공급자는 토큰 예산 안내
- 이번 검증과 결과: `TestBudgetHoldsNewAIWork`, `TestUsageUnknownCost` 포함 `go test -race ./...` 통과, 프론트엔드 22건 통과
- 미검증·알려진 제한: 진행 중인 요청은 한도와 무관하게 끝까지 진행(명세대로 절대 상한 아님)
- 다음에 실행할 구체적인 작업: 재시작 시 interrupted 처리·취소의 실제 앱 시험(T16·T17)

- 현재 단계: M2 / 재시작·취소 실제 앱 시험
- 구현 완료: 내장 테스트 연결에 `auto-slow`(20초 후 결과, 즉시 취소 가능) 시나리오 추가. 대시보드가 불러오기 전에 새 서비스 양식을 여는 버그, 실행 기록 회차 칸 앞 '·' 표시 버그 수정
- 이번 검증과 결과: 실제 앱(`wails dev`, 새 데이터 폴더)에서 기획 AI(auto-slow) 실행 중 앱 프로세스를 `kill -9`로 강제 종료 → DB에 running으로 남음 → 같은 데이터 폴더로 재시작 시 잠금 정상 획득, 기획은 자동 재실행 없이 interrupted(이유 표시), 대시보드에 '막힘 1' → '다시 시도'로 시도 2 실행 → 실행 취소 시 2초 안에 실행·기획 모두 '취소됨', 되돌리지 않았다는 안내 표시. 프론트엔드 23건·Go 전체 통과
- 미검증·알려진 제한: 실제 Codex·Claude 프로세스 취소(T17 실기)는 게이트에서 사용자 승인 후
- 다음에 실행할 구체적인 작업: M2 게이트 — 실제 AI로 기획 → 승인 → 개발 → 사람 리뷰 → 검증 (사용자 승인 필요)

- 현재 단계: M2 게이트
- 구현 완료: 서비스 작업 폴더 설정(절대 경로·존재하는 폴더만), Claude bridge — 작업·결과 폴더 안 파일 쓰기는 자동 허용(쓰기 경로 훅과 같은 판정, Bash 등은 계속 승인), 승인 표시 500자 제한, 공급자 주 프로세스가 스스로 끝나도 프로세스 그룹 정리
- 이번 검증과 결과 (실제 앱 `wails dev`, 새 데이터 폴더, 작업 폴더는 한글·공백 경로):
  - 연결: Codex(codex-cli 0.160.0, ChatGPT 로그인, 모델 gpt-6.1-sol)·Claude(Agent SDK, 개인용 claude.ai 로그인, claude-opus-5-5) 모두 실행 파일→버전→인증→실제 호출 4단계 통과, '사용 가능'
  - 흐름 "슬러그 함수 개발": 기획(Claude) → 기획 승인(나) → 개발(Codex: slug.py·test_slug.py 작성, 결과 폴더 쓰기 승인 요청 없음, 앱 검증 명령 unittest 8개 통과) → 코드 리뷰(나, 코드·테스트 직접 확인 후 통과) → 검증(Claude: 기준별 근거 JSON 보고, 검증 명령 통과) → 실행 완료
  - 결과는 artifacts/에 읽기 전용 고정, 해시 일치. 사용량 기록(Codex 비용 미보고는 별도 표시)
  - 도구 승인 2건: 수정 전 Claude 결과 파일 쓰기(→ 수정), Claude의 Bash(ls·cat·unittest, 사람이 허용)
  - 실제 취소: Codex 즉시 cancelled·남은 프로세스 없음 / Claude는 처음에 bridge 종료 후 SDK가 띄운 프로세스가 몇 초 남는 결함 발견 → 가짜 bridge 재현 테스트 작성 후 수정 → 재시험 통과
- 미검증·알려진 제한: Windows는 주 프로세스 종료 후 taskkill /T로 하위 트리를 찾을 수 없어 Job Object 필요(M6). 실제 API 키(Claude·OpenAI API) 시험 없음. 브라우저 자동화의 좌표 클릭이 간헐적으로 버튼에 닿지 않아 일부 조작은 요소 참조·JS 클릭으로 진행(앱 동작은 정상 확인)
- 마지막 관련 코드/테스트: `runners/claude/src/bridge.ts`, `internal/providers/{process,claude}.go`, `real_cancel_test.go`, `codex_unix_test.go`
- 다음에 실행할 구체적인 작업: M2 게이트 보고 후 사용자 확인 → M3 AI 설계

### 2026-10-07 (M3)
- 현재 단계: M3 / 설계 백엔드 (브랜치 `feat/m3-ai-design`)
- 구현 완료: migration 0003(서비스별 auto 허용 범위, 설계 요청 기록; v2→v3 업그레이드 시험), `internal/design` — 설계안 형식(엄격 파싱, 도구·권한·키 필드 불가), 검증·정규화(같은 이름 역할은 재사용, 사람은 local-owner, 쓸 수 없는 연결은 비우고 부족 항목으로 표시, 사람이 맡겠다고 한 업무가 실제 사람 업무인지 코드로 확인, 흐름은 수동 흐름과 같은 검증), 변경 비교(재사용·신규 역할, 기존 역할 수정 제안은 표시만, 담당자, 업무, 부족 항목), 설계 AI 실행(도구 요청 거절·질문 자동 응답, 10분 제한), 한 트랜잭션 적용(`ApplyDesign`), auto 모드 사전 허용 범위 검사(시작 가능·예산 지정·허용 연결·부족 항목 없음) 후 적용→버전 확정→시작, 이중 적용 방지
- 이번 검증과 결과: `go test -race ./...` 통과 — T02(리뷰 local-owner 배정, 기존 역할 재사용), T03(auto 자동 시작 후 사람 승인 대기), 허용 범위 밖 auto는 아무것도 만들지 않음, 잘못된 설계안 실패 처리
- 미검증·알려진 제한: 실제 설계 AI 호출은 게이트에서(사용자 승인 필요)
- 다음에 실행할 구체적인 작업: 목표 요청 화면과 bindings

- 현재 단계: M3 / 목표 요청 화면
- 구현 완료: bindings(설계 시작·목록·조회(준비된 설계는 현재 조직·연결 기준 재검사, auto 시작 불가 이유 포함)·적용·버리기, 서비스 auto 허용 연결 설정), 설계 AI는 앱 종료 시 함께 취소. 화면: '목표 요청' 메뉴 — 목표·대상(새/기존)·내가 맡을 업무·모드(auto는 예산·허용 연결)·설계 연결 선택, 실행은 두 번 눌러 확인, 설계 중 2초마다 갱신, 설계안 보기(재사용·새 역할, 기존 역할 수정 제안은 '자동으로 적용되지 않음', 담당자·연결, 업무별 담당·사람 표시, 부족 항목, 검증 문제, auto 시작 불가 이유), 적용하고 편집하기→흐름 편집기 / 적용하고 자동 시작→실행 기록 / 버리기. 서비스 개요에 자동 실행 허용 범위
- 이번 검증과 결과: Go 전체(`TestDesignBindings` 포함)·프론트엔드 26건·build 통과
- 다음에 실행할 구체적인 작업: M3 게이트 — 실제 설계 AI로 "게임 출시, 리뷰는 내가" (사용자 승인 필요)

### 2026-10-08 (M3 게이트 중 수정)
- 현재 단계: M3 게이트 (실제 Codex 설계)
- 구현 완료: ① 설계 프롬프트에 실제 결과 파일 경로를 넣음(CLI 에이전트는 프롬프트만 보므로 경로가 없으면 파일을 쓰지 못함 — 첫 실제 설계가 이 결함으로 실패), 응답 텍스트의 ```json 코드 블록도 설계안으로 인정, 실패 오류에 마지막 응답 일부 기록 ② 앱이 종료되면 '설계 중'으로 영원히 남던 설계 요청을 시작 시 '실패(앱 종료로 중단)'로 정리 ③ 설계 AI에 파일 읽기·명령 실행이 필요 없다고 명시
- 이번 검증과 결과: 재현 테스트(`TestRunTellsAgentWhereToWrite`, `TestRunAcceptsFencedJSONReply`, `TestRecoverMarksDraftingFailed`) 수정 전 실패 확인 후 통과. 실제 앱 재시작 시 멈춰 있던 설계가 실패로 정리됨 확인
- 진단 기록: 두 번째 실제 설계가 14분간 끝나지 않아 앱에 SIGQUIT을 보내 고루틴 덤프를 확보 — 설계 루프는 정상 동작 중이었고, Mac 잠자기로 Go 단조 시계가 멈춰 10분 제한이 아직 도달하지 않은 상태로 판단. 이 진단으로 앱이 종료되며 결함 ②를 발견
- 추가 수정 ④: 세 번째 실제 설계가 `notes`를 문자열 목록으로 보내 실패 → 표시용 메모는 목록도 받아 줄바꿈으로 합침(`Text`), 형식 오류로 실패한 설계도 원래 답변을 보관하고 '저장된 답변 다시 검사 (AI 호출 없음)' 버튼 추가 ⑤ 설계가 끝나도 요청 기록 목록이 '설계 중'으로 남던 화면 문제 → 상세 상태가 바뀌면 목록 갱신 (`Design.test.tsx` 수정 전 실패 확인)
- 게이트 결과(2026-10-08, 실제 Codex 설계 4회 호출 — 1~3회는 위 결함으로 실패): 4회째 설계안 '검토 대기', 기존 역할 4개 모두 재사용, '코드 리뷰' → 사람(local-owner), AI 담당자는 사용 가능한 연결(내 Claude·내 Codex)만 사용. '적용하고 편집하기' → 새 서비스 '톡톡' 생성, 흐름 편집기에서 버전 1 확정. DB 확인: 역할 이름별 1개(중복 없음), 설계 요청 상태 applied. 배포 위치·절차가 없어 '부족한 항목'으로 실행 가능(canStart)은 false — 의도대로 실행 전 보류
- push: `gh` 로그인(`workflow` 권한)으로 `feat/m3-ai-design` push. 첫 CI(run 37646913610)에서 Windows만 `TestArtifactsAreFrozenCopies` 실패 — 테스트가 경로 끝을 `"/out"` 문자열로 비교한 결함(엔진은 정상) → `filepath.Base`로 수정 후 run 37649324901에서 3 OS 모두 테스트·빌드 통과, 빌드 산출물 3개 업로드
- 다음에 실행할 구체적인 작업: M3 게이트 보고 후 사용자 확인 → M4 협업
