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
- [ ] Codex 취소 실제 확인 — 가짜 서버로만 확인, ⚠️ 실제 미검증
- [x] Claude SDK bridge (TS, JSONL) 최소 실행 — 실제 SDK 호출 성공 (2026-10-03)
- [ ] Claude 취소 실제 확인 — 가짜 bridge로만 확인, ⚠️ 실제 미검증

**빌드**
- [x] macOS `wails build` 성공
- [ ] GitHub Actions로 Windows·Linux 빌드 — `.github/workflows/build.yml` 작성, ⚠️ 원격 실행 전(브랜치 push 필요)
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
- [ ] 3단 레이아웃 + 서비스 대시보드 (생성·전환·보관)
- [ ] 조직·역할·배정 화면, 합성 지침 미리보기
- [ ] 흐름 편집 기본 (React Flow: 노드·연결·담당자)
- [ ] 내 할 일·승인함, 결과·기록 화면
- [ ] 🚪 게이트: 6.2 예시 흐름 끝까지 실행 + 두 서비스 분리 (T01·T07)

### M2 실제 AI 실행
- [ ] Codex 연결 완성: 이벤트 정규화·승인/질문 응답·사용량
- [ ] Claude bridge 완성: canUseTool·훅·질문·취소
- [ ] OpenAI Responses·Claude Messages API 연결 (구조화 출력)
- [ ] 연결·설정 화면: 탐지·버전·인증·호출 성공 분리 표시 (T04)
- [ ] 도구 승인 (T12), 사용량·비용·예산 (T19)
- [ ] 재시작 시 interrupted 처리·취소 확인 (T16·T17)
- [ ] 🚪 게이트: 실제 기획 → 승인 → 개발 → 사람 리뷰 → 검증

### M3 AI 설계
- [ ] 설계 요청 → JSON 초안 스키마·검증 (권한·키 자동 생성 금지)
- [ ] 기존 역할 재사용/신규 추가 diff, 배정 생성
- [ ] review·auto 모드, 사전 허용 범위 검증 (T03)
- [ ] 목표 요청 화면 (새/기존 서비스, 변경 비교)
- [ ] 🚪 게이트: "게임 출시, 리뷰는 내가" → local-owner 배정 (T02)

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
