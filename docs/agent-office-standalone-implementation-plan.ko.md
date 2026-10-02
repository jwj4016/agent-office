# Agent Office 독립 개발 계획서

문서 버전: 1.0 / 작성일: 2026-10-02  
대상: 이 대화와 이전 계획서를 읽지 않은 개발자 또는 AI 개발 에이전트  
제품: Windows·macOS·Linux용 Go + Wails 데스크톱 앱

**이 파일 하나로 제품 요구사항과 개발 순서를 파악할 수 있어야 한다.** 이전 대화, 기존 시안, 별도 디자인 파일은 필수 입력이 아니다. 공식 기술 문서는 구현 시 API와 호환 버전을 확인하는 참고 자료다.

이 문서는 설계 명세다. 실제 앱 코드·설치 파일·AI 연결이 완성되었다는 뜻은 아니다. 기존 구현이 제공되지 않았다면 새 저장소에서 시작하고, 구현이 있으면 먼저 확인해 재사용한다.

## 빠르게 읽는 제품 요약

사용자는 회사의 역할과 업무 프로세스를 구성하고, 업무를 AI 또는 사람에게 배정한다. 앱은 결과를 다음 업무로 전달하고 협의·검토·수정·승인을 관리한다. 귀여운 도트 사무실은 실제 업무 상태를 표현한다.

필수 기능:

1. **직접 구성:** 사용자가 역할·하위 역할·지침·업무 흐름을 만들면 담당 AI가 프로세스대로 실행한다.
2. **AI 구성:** “게임을 출시하고 싶어” 같은 요청에서 AI가 역할·지침·프로세스·담당자를 만든다.
3. **사람과 협업:** 코드 리뷰 등을 사용자가 직접 맡고 AI에게 수정 요청을 전달한다.
4. **서비스 분리:** 게임 서비스와 부동산 분석 서비스를 별도 프로젝트로 관리한다.
5. **도트 사무실:** 누가 작업·회의·검토·대기 중인지 실제 실행 기록과 연결한다.
6. **AI 연결:** Claude와 Codex, OpenAI·Claude 모델 API를 업무에 따라 사용한다.

첫 실제 사용자 여정:

    서비스 프로젝트 생성
    → 기획 AI → 기획 승인(사용자) → 설계 AI
    → 백엔드·프론트엔드 AI → 통합 AI
    → 코드 리뷰(사용자) → QA AI → 결과 전달

코드 리뷰 수정 요청은 개발 AI에게 돌아간다. “코드 리뷰는 내가 할게”라는 자연어 요구도 담당자 배정에 반영한다.

**초기 범위:** 개인 PC 중심, 사용자 본인 1명과 여러 AI, 여러 서비스 프로젝트. 회원가입은 필수가 아니다. 여러 사람의 원격 공동 작업, 클라우드 동기화, 원격 실행 서버, 공개 템플릿 마켓은 후속 범위다.

---

## 1. 확정된 결정과 구현 기본값

| 항목 | 결정 |
|---|---|
| 데스크톱 | Go + Wails v2 안정 계열. Electron·Tauri로 변경하지 않는다 |
| 화면 | React + TypeScript + Vite |
| 업무 편집 | React Flow |
| 도트 화면 | Canvas 2D + CSS. 일반 폼·목록은 React |
| 실행·파일·프로세스 관리 | Go |
| 저장 | SQLite + database/sql. 초기 드라이버는 modernc.org/sqlite로 실증 |
| 비밀정보 | OS 비밀 저장소. Go 연결 후보는 zalando/go-keyring |
| AI 코딩 연결 | Codex App Server의 로컬 stdio, Claude Agent SDK 실행 프로그램 |
| 일반 AI 업무 | OpenAI Responses API, Claude Messages API |
| Claude SDK 실행 프로그램 | TypeScript + Node.js. 필요한 SDK 기능만 연결 |
| 코드 작업 | Git worktree 분리, 통합 작업 공간에서 검증 |
| UI 언어 | 한국어 우선. 문구를 분리해 추가 언어 확장 가능 |
| 자동 모드 | 기본은 검토 후 실행. 사전 허용 범위 내 자동 구성·실행도 지원 |
| 동시 실행 | 앱 전체 활성 AI 업무 기본 2개. 서비스별 제한도 적용 |
| 반복 제한 | 기본 수정 회차 최대 3회, 안전한 네트워크 재시도 최대 2회 |
| 기본 사람 | 로컬 사용자 ID: local-owner. 실제 업무와 승인 모두 배정 가능 |
| 실제 공개 | 초기에는 배포 패키지·안내. 검증된 배포 연결부터 자동화 |

modernc.org/sqlite는 CGO가 필요 없는 SQLite 드라이버다. Wails 자체의 플랫폼 의존성까지 없어지는 것은 아니다. [드라이버 문서](https://pkg.go.dev/modernc.org/sqlite)

OS 비밀 저장소가 없으면 키를 세션 메모리에만 보관한다. 일반 JSON·SQLite·텍스트 파일에 평문 키를 저장하지 않는다. [Go keyring 프로젝트](https://github.com/zalando/go-keyring)

Go·Node·Wails·공급자 SDK 버전은 첫 실증 때 함께 확인하고 go.mod, go.sum, package-lock.json 및 도구 버전 기록에 고정한다. 이 문서의 작성 시점 버전을 무조건 최신 호환 조합으로 가정하지 않는다.

성능 목표는 상시 자원 부담을 줄이는 것이다. Go로 작성했다고 AI 생성 속도가 빨라지거나 전체 메모리 사용이 자동으로 줄어든다고 보장하지 않는다.

## 2. 제품 개념과 서비스 분리

### 2.1 관리 계층

    회사 Organization
    ├─ 공통 역할 Role 및 회사 지침
    ├─ 서비스 프로젝트 Project: 게임 서비스
    │  ├─ 담당자 배정 Assignment
    │  ├─ 업무 흐름 Workflow: 개발 / 출시 / 업데이트
    │  └─ 실행 Run → 업무 시도 StepAttempt → 결과 Artifact
    └─ 서비스 프로젝트 Project: 부동산 분석 서비스
       ├─ 담당자 배정 Assignment
       ├─ 업무 흐름 Workflow: 서비스 개발 / 자료 검토 / 분석 보고서
       └─ 독립적인 실행과 결과

서비스 하나를 프로젝트 하나로 관리한다. 역할은 책임·지침이고, 담당자는 역할을 수행하는 AI 또는 사람이다. 조직 계층은 지침·보고 관계를 정하며 실행 순서는 업무 의존 관계가 정한다.

첫 버전은 기본 회사 1개를 제공한다. 프로젝트는 여러 개 생성할 수 있다. 회사 공통 역할은 프로젝트마다 다른 담당자와 설정으로 재사용한다.

### 2.2 반드시 분리할 항목

프로젝트별 목표, 담당자 배정, 업무 지침, 자료, 대화·기억, AI 세션, 업무 흐름, 예산, 코드 저장소, 결과, 승인, 사무실 선택 상태를 분리한다.

모든 조회·수정·실행 API는 대상 프로젝트를 확인한다. 요청에 임의로 섞인 다른 프로젝트의 입력·세션·산출물 ID를 거절한다. 역할 이름이 같다는 이유로 다른 프로젝트의 AI 대화를 이어가지 않는다.

회사 공통 역할을 바꿔도 진행 중 실행의 지침은 바뀌지 않는다. 자료 공유는 사용자가 선택한 산출물 버전을 별도 프로젝트로 가져오고 출처를 남기는 방식으로 시작한다. 폴더 전체나 AI 기억을 자동 공유하지 않는다.

서비스 전환은 화면 선택만 바꾼다. 실행은 유지한다. 일시 정지·취소는 지정한 실행에만 적용한다. 전체 대시보드와 내 할 일은 프로젝트 이름·업무·상태를 함께 표시한다.

### 2.3 역할·담당자 규칙

역할 필드: 이름, 설명, 상위 역할, 책임, 지침, 기본 산출물, 자료 접근 정책, 도구 정책, 외형.

담당자 배정 필드: 프로젝트, 역할, 종류 ai 또는 human, 표시 이름, AI 연결·모델, 프로젝트 지침 변경, 외형 변경.

AI 배정에는 유효한 공급자 연결이 필요하다. 사람 배정에는 local-owner를 사용한다. 역할·프로젝트 배정을 만들 때 모델 프로세스를 시작하지 않는다.

지침 표시 순서: 회사 → 상위 역할 → 본인 역할 → 프로젝트 → 업무 → 이번 수정 의견. 합쳐진 최종 지침을 미리 보여준다. 조직의 순환 관계는 저장하지 않는다.

실제 권한은 구조화된 실행 정책으로 관리하며 상위 제한을 하위 역할이 확대하지 못한다. 프롬프트의 금지 문구만으로 쉘·파일 권한을 제어하지 않는다.

## 3. 제품 요구사항

| ID | 요구사항 | 사용자가 할 수 있어야 하는 일 |
|---|---|---|
| R01 | 서비스 관리 | 서비스 생성·전환·보관, 독립적인 흐름과 실행 관리 |
| R02 | 조직 관리 | 공통 역할·하위 역할·지침, 프로젝트 담당자 생성·편집 |
| R03 | 수동 흐름 설계 | 업무 추가·삭제·연결, 담당자·입력·결과·조건 지정 |
| R04 | AI 조직·흐름 설계 | 자연어 요청으로 역할·배정·업무 초안 생성 및 적용 |
| R05 | AI 실행 | 지정된 공급자로 업무 실행, 이벤트·산출물·오류 확인 |
| R06 | 사람 업무 | 자료와 변경 내역을 확인하고 결과·리뷰 의견 제출 |
| R07 | 승인·반려 | 결과 버전별 승인·수정 요청·보류, 관련 업무 재작업 |
| R08 | 협업 | 담당자 간 질문·답변·검토·결정을 업무에 연결 |
| R09 | 개발 통합 | worktree 분리, 통합·충돌 처리, 테스트·빌드 |
| R10 | 사무실 | 실제 상태를 캐릭터·회의·배지로 표시 |
| R11 | 복구·제한 | 재시작 복원, 취소, 사용량·예산·횟수 제한 |
| R12 | 내보내기·배포 | 키 제외 내보내기, 세 OS 설치 및 주요 동작 검증 |

### 3.1 AI가 조직과 흐름을 만드는 동작

입력: 대상 프로젝트 또는 새 프로젝트 요청, 목표, 현재 조직·연결, 사람이 맡을 업무, 허용 도구·예산·범위.

출력: 필요한 신규 역할, 프로젝트 배정, 업무 흐름, 산출물 정의, 사람 업무와 승인 지점, 부족한 연결·자료 목록.

AI 출력은 JSON으로 받고 구조·의존 관계·배정·프로젝트 범위를 검증한다. 모델이 제안한 도구·권한·API 키를 자동 생성하거나 허용하지 않는다.

- review 모드: 초안 → 사용자가 편집 → 버전 확정 → 시작.
- auto 모드: 사전 허용 범위 검증 → 초안 적용과 배정 생성 → 버전 확정 → 자동 시작.
- 연결·필수 자료가 없거나 범위가 불명확하면 문제를 표시하고 관련 작업을 시작하지 않는다.
- auto라도 사람 업무와 지정 승인 지점을 건너뛰지 않는다.
- 새 역할은 기존 역할을 먼저 재사용하고 없을 때 추가한다. 기존 회사 역할의 수정은 별도 변경 사항으로 보여준다.
- 현재 실행에 대한 변경은 새 버전·새 실행으로 적용한다. 진행 중 그래프를 직접 수정하지 않는다. 초기에는 기존 결과의 자동 이관을 하지 않고 필요 결과를 명시적으로 연결한다.

### 3.2 사람 업무는 승인과 별개다

사람에게 task 또는 review를 배정할 수 있다. 필요한 입력, 파일·코드 변경, 검증 결과와 완료 기준을 할 일에 제공한다.

사람은 결과 파일·텍스트·의견을 제출하고 완료한다. review는 통과 또는 수정 요청을 선택한다. 제출하지 않은 작업을 AI가 대신 끝낸 것으로 처리하지 않는다. 무응답은 승인·완료가 아니다.

리뷰 통과 의견은 제출 내용과 확인한 결과 버전을 포함한 report 산출물로 저장한다. 따라서 후속 QA가 사람이 완료한 리뷰를 입력으로 받을 수 있다.

코드 리뷰 수정 요청에는 대상 개발 업무와 이유를 지정한다. 그 결과가 바뀌면 사용자는 최신 변경과 테스트 결과를 다시 확인한다. 할 일에는 프로젝트·실행·시도 버전을 표시한다.

## 4. 앱 구조와 파일 구성

하나의 저장소, 하나의 데스크톱 제품으로 시작한다. 처음부터 별도 클라우드 서버나 메시지 브로커를 만들지 않는다.

    React 화면
      ↕ 제한된 Wails bindings / events
    Go App API
      ├─ 조직·프로젝트·흐름 관리
      ├─ 업무 엔진과 스케줄러
      ├─ 공급자 연결 / Claude SDK 실행 프로그램
      ├─ Git 작업 공간·검증
      └─ SQLite·산출물·OS 비밀 저장소

엔진은 Go 백그라운드 작업으로 수행하고 화면 호출에서 장시간 블로킹하지 않는다. 외부 CLI와 SDK는 관리되는 하위 프로세스로 실행한다. DB를 상태의 기준으로 삼고 저장이 확정된 이벤트만 화면에 전달한다.

권장 파일 구성:

    main.go
    app.go
    wails.json
    go.mod / go.sum
    internal/
      domain/          데이터·상태·검증 규칙
      engine/          스케줄링·수정 회차·복구
      providers/       Codex·Claude·모델 API·테스트용 provider
      storage/         SQLite 접근·migrations·백업
      workspace/       Git·파일·산출물 검증
      secrets/         OS 비밀 저장소
    frontend/
      src/             화면·업무 편집·도트 사무실
      wailsjs/         Wails 생성 bindings
      package.json / package-lock.json
    runners/claude/    TypeScript SDK bridge
    tests/fixtures/   공급자 이벤트·흐름·오류 예시
    docs/
      progress.md
      decisions.md
      environment.md
      acceptance.md

패키지 수를 형식적으로 늘리지 않는다. 위 구분은 책임의 기준이며 파일은 필요한 만큼 만든다.

Wails는 Go 메서드의 화면 호출 bindings와 양방향 이벤트를 제공한다. 임의 쉘이나 파일 경로를 받아 그대로 실행하는 범용 메서드를 노출하지 않는다. [bindings](https://v2.wails.io/docs/howdoesitwork/), [events](https://v2.wails.io/docs/reference/runtime/events/)

## 5. 데이터 저장 계약

ID는 충돌 가능성이 낮은 문자열로 생성하고 표시 이름과 구분한다. 시간은 UTC ISO 8601로 저장하고 화면에서는 사용자 설정으로 표시한다. 비밀값 대신 secretRef만 DB에 기록한다.

| 엔터티 | 최소 저장 내용 |
|---|---|
| Organization | id, name, instructions, policy |
| Role | id, organizationId, parentRoleId, name, mission, instructions, outputDefaults, policy, appearance |
| Project | id, organizationId, name, goal, instructions, workspacePath, budget, mode, status |
| Assignment | id, projectId, roleId, actorKind, actorId, connectionId, model, overrides |
| ProviderConnection | id, provider, executablePath, secretRef, config, verifiedCapabilities |
| Workflow | id, projectId, title, draftJson, revision |
| WorkflowVersion | id, workflowId, number, specJson, assignmentsSnapshot, policySnapshot, createdAt |
| Run | id, projectId, workflowVersionId, status, limits, startedAt, endedAt |
| StepAttempt | id, runId, stepId, revisionRound, attempt, generation, status, inputManifest, providerSession, workspaceId |
| Artifact | id, projectId, runId, stepAttemptId, outputKey, version, type, path, hash, validity |
| Message | id, projectId, runId, stepAttemptId, sender, recipient, kind, body, artifactRefs, replyTo |
| Approval | id, runId, stepAttemptId, generation, targetManifest, decision, reason, decidedAt |
| ExecutionEvent | id, sequence, projectId, runId, stepAttemptId, kind, payload, createdAt |
| Workspace | id, projectId, runId, assignmentId, path, baseCommit, branch, status |

이 표는 책임과 관계의 기준이다. 구현에 필요하지 않은 중간 테이블은 늘리지 않는다.

초기 DB 구현은 실행·승인·산출물 참조에 외래키와 유일성 제약을 적용한다. 흐름과 스냅샷은 검증된 JSON으로 저장해도 된다. 상태 변경과 이벤트 추가는 같은 트랜잭션으로 처리한다.

필수 제약:

- workflowId와 assignmentId는 지정 프로젝트에 속해야 한다.
- 같은 업무 시도의 완료·승인 결정은 한 번만 적용된다.
- providerSession은 projectId·runId·assignmentId·stepAttemptId에 연결된다.
- 한 실행에서 입력으로 사용할 산출물 버전·해시는 시작 시 고정된다.
- 폐기된 결과와 이전 시도의 늦은 응답은 현재 상태를 진행시키지 않는다.

Run에는 업무별 현재 generation을 기록한다. StepAttempt의 generation은 생성 후 고정한다. 재작업 시 Run의 해당 업무 generation을 높이고 새 시도를 생성해 이전 시도와 구분한다.

SQLite는 migration 번호를 기록한다. 첫 구현은 DB 쓰기를 직렬화하고 트랜잭션을 짧게 유지한다. 백업은 실행 상태를 일관되게 멈추거나 SQLite 백업 방식을 사용한다. 열린 WAL DB의 본체 파일만 복사하지 않는다.

산출물은 프로젝트별 파일로 저장한다. 메타데이터는 DB에 저장하고 파일 크기·존재·허용 경로를 검증한다. 키·로그인 파일·토큰은 프로젝트 내보내기에서 제외한다.

## 6. 업무 흐름 데이터 계약

정상 진행은 방향이 있는 비순환 그래프(DAG)다. 반려는 일반 연결선의 순환 대신 별도 수정 회차로 처리한다.

### 6.1 노드 필드

| 필드 | 의미 |
|---|---|
| id, title | 흐름 안에서 고유한 ID와 표시명 |
| kind | task / review / approval / condition / join |
| assignmentId | task·review의 AI 또는 사람, approval은 사람. condition·join은 없음 |
| dependsOn | 완료되어야 할 선행 노드 ID 배열 |
| inputs | name, fromStep, outputKey, required로 정의한 결과 연결 |
| instructions | 이번 업무 지시 |
| outputs | key, type, required, 선택적 JSON schema |
| completion | 결과 존재·스키마·검증 명령 등 실제 완료 기준 |
| reworkTargets | 반려 시 돌아갈 수 있는 상위 업무 ID |
| limits | 시간·수정·재시도·토큰 제한 |
| routing | condition의 허용 비교 규칙과 출력 경로 |

type은 markdown, json, file, code_change, report 중 하나로 시작한다. 코드 결과는 변경 기준 커밋·변경 목록·테스트 정보를 포함한다.

task와 review의 담당자는 AI 또는 사람이다. approval은 사람의 결정만 받는다. join은 산출물 생성 없이 선행 결과를 합류시키는 엔진 노드다.

condition은 지정된 JSON 결과의 fieldPath에 eq, ne, in, exists 비교를 적용한다. 모델 응답이나 사용자 문자열을 JavaScript·쉘 코드로 평가하지 않는다. 경로에 오류가 있으면 실패 처리하고, 값이 비교와 일치하지 않으면 명시한 기본 경로를 선택한다.

routing은 source={fromStep, outputKey, fieldPath}, branches=[{operator, value, targetStep}], defaultTarget, joinStep으로 정의한다. 첫 번째로 일치하는 분기 하나를 선택한다. exists는 value를 생략할 수 있다. source는 선행 JSON 결과를 참조하고, targetStep·defaultTarget은 같은 흐름의 시작 노드를, joinStep은 같은 흐름의 join 노드를 가리킨다.

condition의 대상 노드는 조건 노드를 선행으로 참조한다. 분기는 합류 노드에서 다시 만나는 구조로 제한한다. 첫 버전에서 분기 내부의 추가 분기는 허용하지 않는다. 선택되지 않은 분기의 노드는 skipped로 처리한다. 외부 활성 선행 노드를 가진 노드를 무조건 skipped로 바꾸지 않도록 그래프 검증 시 모호한 분기 연결을 거절한다.

### 6.2 최소 예시

아래 JSON은 흐름 초안의 형식 예시다. 배정 ID는 같은 프로젝트에서 미리 만들고 연결한다. 예시 필드의 생략 기본값은 inputs=[], reworkTargets=[], limits=프로젝트 기본값, outputs.required=true, completion=필수 결과 검증이다.

    {
      "schemaVersion": 1,
      "title": "서비스 개발",
      "nodes": [
        {
          "id": "plan", "title": "기획",
          "kind": "task", "assignmentId": "a-planner",
          "dependsOn": [], "instructions": "요구사항과 완료 기준을 작성한다.",
          "outputs": [{"key": "spec", "type": "markdown"}]
        },
        {
          "id": "approve", "title": "기획 승인",
          "kind": "approval", "assignmentId": "a-owner",
          "dependsOn": ["plan"],
          "inputs": [{"name": "기획서", "fromStep": "plan", "outputKey": "spec", "required": true}],
          "reworkTargets": ["plan"], "outputs": []
        },
        {
          "id": "design", "title": "구조·API 계약",
          "kind": "task", "assignmentId": "a-architect",
          "dependsOn": ["approve"],
          "inputs": [{"name": "기획서", "fromStep": "plan", "outputKey": "spec", "required": true}],
          "outputs": [{"key": "contract", "type": "markdown"}]
        },
        {
          "id": "backend", "title": "백엔드 개발",
          "kind": "task", "assignmentId": "a-backend",
          "dependsOn": ["design"],
          "inputs": [{"name": "API 계약", "fromStep": "design", "outputKey": "contract", "required": true}],
          "outputs": [{"key": "change", "type": "code_change"}]
        },
        {
          "id": "frontend", "title": "프론트엔드 개발",
          "kind": "task", "assignmentId": "a-frontend",
          "dependsOn": ["design"],
          "inputs": [{"name": "API 계약", "fromStep": "design", "outputKey": "contract", "required": true}],
          "outputs": [{"key": "change", "type": "code_change"}]
        },
        {
          "id": "integrate", "title": "통합",
          "kind": "task", "assignmentId": "a-architect",
          "dependsOn": ["backend", "frontend"],
          "inputs": [
            {"name": "백엔드", "fromStep": "backend", "outputKey": "change", "required": true},
            {"name": "프론트엔드", "fromStep": "frontend", "outputKey": "change", "required": true}
          ],
          "outputs": [{"key": "integrated", "type": "code_change"}]
        },
        {
          "id": "review", "title": "내 코드 리뷰",
          "kind": "review", "assignmentId": "a-owner",
          "dependsOn": ["integrate"],
          "inputs": [{"name": "통합 변경", "fromStep": "integrate", "outputKey": "integrated", "required": true}],
          "reworkTargets": ["backend", "frontend"],
          "outputs": [{"key": "review", "type": "report"}]
        },
        {
          "id": "qa", "title": "검증",
          "kind": "task", "assignmentId": "a-qa",
          "dependsOn": ["review"],
          "inputs": [
            {"name": "리뷰", "fromStep": "review", "outputKey": "review", "required": true},
            {"name": "통합 변경", "fromStep": "integrate", "outputKey": "integrated", "required": true}
          ],
          "outputs": [{"key": "verification", "type": "report"}]
        },
        {
          "id": "deliver", "title": "결과 전달",
          "kind": "task", "assignmentId": "a-planner",
          "dependsOn": ["qa"],
          "inputs": [{"name": "검증", "fromStep": "qa", "outputKey": "verification", "required": true}],
          "outputs": [{"key": "delivery", "type": "markdown"}]
        }
      ]
    }

배정: a-planner=기획 AI, a-architect=설계·통합 AI, a-backend=백엔드 AI, a-frontend=프론트엔드 AI, a-qa=QA AI, a-owner=local-owner. 실제 공급자 연결은 사용자 설정 또는 명시적인 테스트 provider로 지정한다.

### 6.3 검증 규칙

ID 고유성, 노드 종류, 담당자 종류, 동일 프로젝트 배정, DAG, 도달 가능한 시작·종료, 입출력 참조, 분기 합류, reworkTargets가 선행 업무인지 검증한다. 연결이 없으면 흐름 저장은 가능하지만 실제 실행 전 부족한 항목을 표시한다.

inputs의 fromStep은 선행 조상이어야 한다. 필수 입력이 비활성 분기의 결과일 수 있는 흐름은 거절하거나 명시적 대체 입력을 요구한다. 첫 버전은 모호한 필수 입력을 거절한다.

## 7. 실행·승인·반려·복구 계약

### 7.1 상태

| 업무 상태 | 의미 |
|---|---|
| pending / ready | 선행 조건 대기 / 시작 가능 |
| running / verifying | 실제 실행 / 완료 기준 검증 |
| waiting_human | 사람이 맡은 업무 결과 대기 |
| waiting_approval | 업무 또는 도구 승인 대기 |
| waiting_input | 질문·추가 자료 대기 |
| succeeded / skipped | 검증 성공 / 선택되지 않은 경로 |
| failed / interrupted / cancelled | 실패 / 불완전 중단 / 사용자 취소 |

프로젝트 안의 실행 상태: running, waiting, paused, succeeded, failed, interrupted, cancelled.

failed 선행 업무의 필수 후속 업무는 ready가 되지 않는다. 다른 독립 업무는 진행할 수 있다. 활성 AI 작업이 있으면 running, 없고 사람·승인·질문을 기다리면 waiting, 기다릴 수 있는 작업 없이 실패로 진행이 막혔으면 failed로 표시한다. 사람이 재시도·수정·취소를 결정할 수 있다.

### 7.2 스케줄링

1. 흐름·배정·정책 스냅샷으로 Run을 생성한다.
2. 선행 완료와 조건을 만족하는 노드를 찾는다.
3. 앱·프로젝트 동시 실행 한도, 예산, 작업 공간 잠금을 확인한다.
4. 업무 시도와 입력 manifest를 저장한 뒤 공급자를 시작한다.
5. 이벤트·질문·승인·산출물을 저장한다.
6. 실제 파일·스키마·검증 결과를 확인하고 succeeded로 전이한다.
7. 다음 업무를 준비한다. 모든 필수 활성 노드가 성공해야 Run이 성공한다.

사람 업무와 승인 대기 자체는 활성 AI 실행 한도를 소모하지 않는다. 도구 승인 중 살아 있는 공급자 프로세스는 별도로 관리해 대기 프로세스가 무제한 늘지 않게 한다.

모델이 “완료”라고 답하거나 프로세스가 종료되었다는 이유만으로 성공하지 않는다. 공급자 성공 상태와 산출물·완료 기준을 모두 확인한다.

### 7.3 승인

업무 결과 승인, 구체적인 도구 승인, 공개·외부 조치 승인을 구분한다. 승인 대상은 runId·stepAttemptId·generation·결과 해시 또는 행위 인수에 고정한다.

같은 승인 요청에 대한 반복 클릭은 이미 확정된 결과를 반환한다. 취소·반려·새 시도 후 도착한 이전 승인 응답은 stale_request로 거절한다. 무응답·창 닫힘·연결 끊김으로 자동 승인하지 않는다.

사전 허용된 프로젝트 작업은 자동 수행할 수 있다. 기획 승인 하나로 임의 배포·파일 삭제·비밀정보 접근까지 허용하지 않는다.

### 7.4 반려와 재작업

사람은 reworkTargets 중 한 개 이상과 구체적인 이유를 선택한다. 선택 대상 및 그 결과에 의존하는 후속 노드가 재작업 범위다.

같은 DB 변경에서 해당 범위의 generation을 증가시키고 현재 결과·승인을 더 이상 최신 유효 결과로 취급하지 않는다. 원본 기록과 파일은 보존한다. 진행 중 범위는 취소를 요청하고 종료 또는 격리를 확인하기 전 새 쓰기 작업을 시작하지 않는다.

새 회차는 영향 범위만 재실행한다. 독립적인 결과는 유지한다. 이전 프로세스의 늦은 이벤트는 이전 시도의 기록으로만 남긴다. 코드 파일을 무조건 이전 버전으로 덮어쓰지 않는다.

조건 결정이 바뀌는 범위는 조건과 그 분기·합류를 다시 평가한다. 기본 수정 회차 3회를 넘으면 사람이 범위·기준·횟수 변경을 결정한다.

### 7.5 일시 정지·취소·종료

- pause: 새 업무 시작을 멈춘다. 진행 중 업무는 계속한다. 사람의 제출은 저장하고 후속 시작은 보류한다.
- cancel: 활성 공급자와 하위 작업 취소를 요청한다. 생성된 파일·외부 행위가 되돌아갔다고 표시하지 않는다.
- UI 서비스 전환: 실행에 영향 없음.
- 앱 종료: 초기 제품은 기록 후 실행을 중단한다. 창을 닫아도 계속 수행하는 기능은 후속으로 둔다.
- 앱 재시작: 기존 running·verifying 및 확인되지 않은 공급자 대기는 interrupted로 표시하고 세션·작업 공간을 확인한다.
- 공급자 재개: 기능과 상태가 확인될 때만 이어간다. 확인할 수 없으면 새로운 시도와 검증 절차를 제안한다.
- 배포·발송처럼 결과가 불명확한 외부 행위는 자동 재시도하지 않는다.

## 8. 에이전트 협업과 컨텍스트

Message.kind는 question, answer, review_request, proposal, decision, handoff, escalation로 시작한다. 발신·수신·관련 업무·산출물 버전을 저장한다.

질문은 replyTo로 연결하고 답변 후 해당 업무의 컨텍스트에 추가한다. AI 수신자가 당장 응답할 수 없으면 대기열에서 처리한다. 서로 기다리는 질문이나 반복 협의는 제한 시간·횟수 후 사용자에게 선택지를 전달한다.

회의는 참여자, 의제, 입력, 최대 발언 수, 결정 담당자, 결과 형식을 가진 협의 작업으로 구현한다. 기본 최대 의견 교환 3회다. 모델 간 동의를 검증 성공으로 대신하지 않는다.

각 AI 입력은 지침 스냅샷, 프로젝트 목표, 현재 업무, 연결된 결과, 관련 결정·질문·수정 의견으로 구성한다. 모든 파일과 전체 대화를 무조건 붙이지 않는다. 요약에는 원본 결과 참조를 남긴다.

새 Run의 세션은 기본 새로 만든다. 제공업체 내부 서브 에이전트는 선택 기능이다. 조직의 하위 역할과 동일시하지 않고, 실제로 제공되는 이벤트만 표시한다.

## 9. 공급자 연결 계약

### 9.1 공통 인터페이스

여러 실제 연결과 테스트 provider가 있으므로 최소 공통 계약을 둔다.

- Capabilities: streaming, toolApproval, question, cancel, resume, usage, coding.
- Start: projectId, runId, stepAttemptId, generation, instructions, inputManifest, outputSpec, workspace, limits, policy, connectionRef.
- Respond: question 또는 approval requestId와 결정.
- Cancel: 특정 실행 시도 취소.
- Resume: 지원되는 세션의 상태 확인 후 재개.
- Events: 정규화된 공개 이벤트 스트림.

기능을 지원하지 않으면 버튼을 비활성화하고 이유를 표시한다. Resume을 지원하지 않는 연결에 가짜 재개를 구현하지 않는다.

공통 이벤트에는 schemaVersion, eventId, sequence, projectId, runId, stepAttemptId, generation, kind, timestamp, payload를 포함한다. payload는 공개 메시지·도구·사용량·결과·오류 등을 구분한다. 원본 프로토콜 데이터는 비밀정보를 제거한 뒤 진단용으로 보관할 수 있다.

공개 메시지와 작업 내역을 표시하며 모델의 숨겨진 사고 과정을 추정해 생성하지 않는다.

### 9.2 Codex

Go에서 사용자 설정의 실행 파일을 시작하고 로컬 stdio JSON 메시지를 교환한다.

기본 수명: initialize → initialized → thread/start → turn/start → 이벤트 수신 → turn/completed 상태 판정. 취소는 해당 threadId·turnId의 turn/interrupt로 요청한다. 승인 요청 ID와 세션을 연결해 응답한다.

개발 착수 시 설치된 버전의 스키마와 실험 기능 상태를 확인한다. 초기 제품에서 원격 WebSocket이나 실험적인 원격 실행에 의존하지 않는다. 공급자 공식 지원 상태가 바뀌면 decisions.md에 영향과 대안을 남긴다. [Codex App Server 공식 문서](https://learn.chatgpt.com/docs/app-server)

### 9.3 Claude

앱의 승인·질문·훅을 연결하는 기본 구성은 TypeScript Claude Agent SDK bridge다. Go와 bridge는 stdin/stdout JSONL로 통신한다. 시작·취소·질문/승인 응답·결과 이벤트를 구분하고 stderr는 별도 로그로 처리한다.

Claude CLI 직접 호출은 별도 제한 모드로 제공할 수 있다. 스트림 읽기만으로 SDK와 같은 승인·훅 제어가 가능하다고 가정하지 않는다.

SDK의 canUseTool은 모든 도구가 항상 거치는 관문이 아니다. 전체 정책 적용은 SDK 훅·실제 도구 설정·실행 환경을 함께 검증한다. 전면 권한 우회를 기본값으로 사용하지 않는다. [SDK 개요](https://code.claude.com/docs/en/agent-sdk/overview), [권한 평가](https://code.claude.com/docs/en/agent-sdk/permissions)

### 9.4 모델 API

Go에서 HTTPS로 OpenAI Responses·Claude Messages를 호출하는 연결을 구현한다. 공식 Go SDK 또는 작은 HTTP 클라이언트 중 실제 요청·스트리밍·취소를 충족하는 구성을 선택한다.

기획·분석·검토·흐름 초안은 구조화 결과를 우선 사용한다. 실제 검색 도구·자료 연결이 없으면 웹 조사 완료로 표시하지 않는다. 자료·출처·기준일·불확실성을 결과에 포함한다. API 호출만으로 로컬 코드 편집·테스트가 자동 실행된다고 가정하지 않는다.

### 9.5 인증·설치

초기 상용 구성은 사용자 API 키 중심이다. CLI 사용자 로그인을 활용하는 옵션은 공급자의 공식 허용 범위 안에서 검증한다. 특히 Claude의 타사 제품에서 구독 로그인·한도 제공은 사전 승인 없는 기본 기능으로 약속하지 않는다. [Claude 인증 안내](https://code.claude.com/docs/en/agent-sdk/overview#get-started)

실행 파일 발견, 버전 호환, 인증, 실제 호출 성공을 따로 표시한다. API 키·로그인 파일을 대화·로그·템플릿에 복사하지 않는다. 사용자 키나 로그인 상태가 없는 경우 개발은 테스트 provider로 진행하고 실제 연결 미검증을 명시한다.

Node.js 및 CLI를 사용자가 설치한 상태로 시작하는 개발 알파는 가능하다. 정식 배포 전 필요한 런타임 탐지·설치 안내 또는 허용되는 내장 배포 방식을 결정하고 각 OS에서 검증한다. 앱 전체가 외부 런타임 없는 단일 바이너리라고 표시하지 않는다.

## 10. 코드 작업·권한·비용

### 10.1 코드 작업

사용자의 원본 저장소와 미커밋 변경을 보존한다. 승인된 기준 커밋에서 개발 담당자별 worktree를 만든다. 동일 경로에 동시에 쓰는 작업을 금지한다.

백엔드·프론트엔드 결과를 통합 작업 공간에 합친 뒤 충돌·테스트·빌드를 처리한다. 사람이 리뷰할 때는 통합된 변경을 제공한다. 저장소 반영은 검토 가능한 패치 또는 명시적인 반영 작업으로 수행한다.

정리는 실행 종료·파일 변경 보존·산출물 추출 후 수행한다. 미반영 변경을 자동 삭제하거나 기존 저장소를 강제로 초기화하지 않는다.

### 10.2 실행 정책

UI에는 키와 임의 쉘을 노출하지 않는다. 모든 bindings는 프로젝트·입력·권한을 검증한다. 외부 자료·모델 응답은 시스템 권한 변경 지시로 해석하지 않는다.

프로세스는 실행 파일·인수·stdin을 구분한다. 사용자 지시를 쉘 문자열에 직접 결합하지 않는다. Windows의 exe·cmd·ps1 설치 방식, macOS PATH, Linux 실행 권한을 각각 처리한다.

프로젝트 밖 경로·심볼릭 링크·산출물 경로 이탈을 검증한다. 자격증명을 빌드·테스트 프로세스에 광범위하게 상속하지 않는다.

worktree와 프로세스 분리는 OS 보안 격리가 아니다. 초기에는 신뢰하는 로컬 저장소를 대상으로 공급자 정책을 검증한다. 구현하지 않은 격리·파일 접근 제한을 지원한다고 표시하지 않는다. 신뢰할 수 없는 코드의 강한 격리는 후속 실행 환경에서 다룬다.

### 10.3 비용

토큰·사용량·추정 비용·알 수 없는 비용을 구분한다. 알 수 없는 금액을 0원으로 표시하지 않는다. 모델·단가는 연결 정보와 갱신 가능한 설정을 사용한다.

예산은 새 요청 시작 전에 확인하고 가능한 진행 요청을 제한한다. 보고 지연과 진행 중 요청 때문에 앱 예산이 제공업체 청구의 절대 상한은 아니다. 배포·발송 등 외부 조치는 일반 네트워크 재시도로 중복 실행하지 않는다.

## 11. 화면과 도트 디자인

| 화면 | 필수 동작 |
|---|---|
| 서비스 대시보드 | 생성·전환·보관, 진행·오류·대기 요약 |
| 조직 | 역할 계층·지침·정책 미리보기, 프로젝트 담당자 |
| 목표 요청 | 새/기존 서비스 선택, 사람 업무, 설계 모드, 생성·변경 비교 |
| 흐름 편집 | 노드·선행·입출력·담당자·수정 대상·조건·버전·실행 |
| 사무실 | 담당자 클릭, 실제 상태·관련 업무·결과·질문 연결 |
| 내 할 일 | 프로젝트별 사람 업무·승인·질문, 제출·수정 요청 |
| 결과·기록 | 문서·출처·코드 변경·검증·사용량·오류 |
| 연결·설정 | 공급자·실행 파일·키·진단·제한·내보내기 |

초기 창은 1280×800 정도를 기준으로 구성하되 작은 창에서도 일반 목록으로 사용 가능하게 한다. 왼쪽 서비스 목록, 중앙 사무실/흐름, 오른쪽 선택 항목 상세 구조를 기본으로 한다.

도트 방향: 따뜻한 크림 배경, 연한 민트·라벤더·살구 포인트, 작은 캐릭터, 책상·모니터·화분·회의실·휴게 공간. 캐릭터는 담당자 인스턴스에 연결하고 AI·사람 배지를 구분한다. 본문·폼은 읽기 쉬운 일반 글꼴을 사용한다.

| 실제 상태 | 표현 |
|---|---|
| 대기 | 고정 좌석의 짧은 대기 동작 |
| running | 모니터 작업과 진행 표시 |
| 협의 작업 | 회의 위치와 참여자 표시 |
| waiting_human / waiting_approval | 사용자 할 일·승인 카드 연결 |
| verifying | 체크리스트 표시 |
| succeeded | 짧은 기쁨 동작·산출물 알림 |
| failed / interrupted | 명확한 상태 아이콘과 복구 동작 |

캐릭터 애니메이션으로 실행 상태를 결정하지 않는다. 사람의 앱 밖 행동은 추정하지 않는다. 모션 줄이기·키보드·상태 텍스트·일반 목록을 제공한다.

숨겨진 사무실은 애니메이션을 멈춘다. 많은 스트리밍 메시지는 약 100ms 단위로 묶어 화면에 반영하되 완료·오류·승인 이벤트는 즉시 반영한다. 로그 목록은 필요한 부분만 표시하고 상세 기록은 저장소에서 읽는다.

측정 시나리오: 서비스 2개, 담당자 총 20명, 흐름 50개 노드, 활성 AI 2개. 이는 테스트 규모이며 검증된 성능 보장이 아니다. 시작 시간·유휴 CPU·전체 프로세스 메모리·화면 반응성을 OS별로 기록한다.

## 12. 초기 템플릿과 실증 범위

- 서비스 개발: 기획 → 사람 승인 → 설계 → 개발 → 통합 → 사람 리뷰 → QA → 전달.
- 게임 출시: 기획·시장·법률 쟁점·비용 분석 → 방향 승인 → 개발·마케팅 준비 → 통합·QA → 출시 승인 → 배포 패키지.
- 법률 검토: 범위·지역·사실 자료 → 근거 조사 → 의견 초안 → 별도 검토 → 사람 결정 → 문서 전달.
- 부동산 분석: 대상·목적 → 자료 확보·품질 점검 → 시장·비용·쟁점 분석 → 비교·검토 → 보고서.
- 빈 흐름: 사용자가 첫 업무부터 직접 생성.

법률·재무 명칭은 AI의 업무 범주다. 최종 판단과 추가 확인을 위한 사람 업무를 배정할 수 있게 한다. 실제 데이터 연결이 없으면 시장 가격·법률 근거를 조사한 것으로 꾸미지 않는다.

첫 코드 실증은 작은 웹 서비스 또는 웹 게임이다. 모든 종류의 대형 게임·상용 서비스를 동일하게 자동 완성한다고 약속하지 않는다. 업무 엔진은 분야에 종속시키지 않는다.

## 13. 개발 단계와 단계별 완료 기준

| 단계 | 구현 항목 | 다음 단계로 넘어가는 기준 |
|---|---|---|
| M0 기술 실증 | Wails 창·bindings·SQLite·키 저장, Codex·Claude 최소 실행·취소, 세 OS 빌드 | 실제/미검증 연결 구분, 저장·취소 확인, 호환 버전과 플랫폼 제약 기록 |
| M1 기본 제품 | 서비스·조직·배정·흐름·버전, 테스트 provider, 사람 업무·승인·반려 | 두 서비스가 분리되고 수동 구성 흐름과 사람 리뷰가 끝까지 동작 |
| M2 실제 실행 | 두 코딩 연결·모델 API, 이벤트·질문·승인·사용량·복구 | 실제 기획 → 승인 → 개발 → 사람 리뷰 → 검증 완료 |
| M3 AI 설계 | 구조화 초안, 역할·배정 생성, review/auto 모드 | 새 서비스 자동 구성, “리뷰는 내가” 반영, 잘못된 흐름·권한 거절 |
| M4 협업 | 질문·협의·결정, worktree·통합·검증, 조건·분기 | 병렬 개발 통합, 수정 회차, 분기 합류 인수 시험 통과 |
| M5 사무실·템플릿 | 도트 연출·상태 연결·접근성·템플릿·내보내기 | 실제 상태와 화면 일치, 모든 초기 템플릿을 편집·실행 가능 |
| M6 출시 검증 | 세 OS 설치·복구·성능·서명·안내·진단 | 지원 범위별 시험 증거와 알려진 제한을 작성하고 출시 결정 |

엔진의 분기·재작업 규칙은 M1에서 설계·테스트하고 복잡한 편집 UI는 M4에서 완성할 수 있다. 테스트 provider 완료는 실제 AI 연결 완료를 의미하지 않는다.

M0는 1~2주를 초기 목표로 한다. 전체 일정은 경험·시험 환경·연결 결과를 확인한 뒤 산정한다. 사용자 키·서명 계정·다른 OS가 없으면 해당 실증을 미검증으로 남기고 독립적인 구현을 계속한다.

### 착수 순서

1. 저장소 유무, 기존 코드, 로컬 지침과 사용 가능한 도구를 확인한다.
2. Wails React+TypeScript 프로젝트를 구성한다.
3. bindings 호출·이벤트 수신, DB 저장·재시작 복원부터 확인한다.
4. 테스트 provider와 사람 리뷰로 6.2 예시 흐름을 실행한다.
5. 실제 Codex·Claude 연결의 승인·질문·취소·세션 구분을 검증한다.
6. 프로젝트 분리·반려·늦은 이벤트 테스트를 통과시킨다.
7. 각 단계의 화면·연결·검증을 완성하며 M6까지 진행한다.

Go·Wails 작업 명령은 각 줄을 개별적으로 실행한다. 아래는 프로젝트 생성 후의 기본 확인 명령이며 설치나 실제 모델 호출을 이미 수행했다는 뜻이 아니다.

    go version
    node --version
    wails version
    wails doctor
    go test ./...
    wails dev
    wails build

frontend에는 test, typecheck, build 스크립트를 정의하고 각각 실행한다. Go race 검사는 해당 플랫폼의 도구가 준비된 환경에서 적용한다. native 앱 자동 조작과 브라우저 UI 테스트는 서로 구분한다.

## 14. 필수 인수 테스트

| ID | 입력·상황 | 기대 결과 |
|---|---|---|
| T01 | 사용자가 역할·흐름 생성 | 배정된 AI가 의존 순서에 맞게 수행 |
| T02 | “게임 출시, 리뷰는 내가” 요청 | 역할·배정·흐름 생성, 리뷰 담당자는 local-owner |
| T03 | auto 모드, 허용 범위 충족 | 자동 확정·시작, 사람 업무·승인은 대기 |
| T04 | 연결 없는 AI 배정 | 저장 가능, 실제 시작은 설명과 함께 차단 |
| T05 | 사람이 리뷰 수정 요청 | 대상 개발·통합·리뷰 후속 재실행, 이전 결과 보존 |
| T06 | 기획 반려 | 기획 의존 범위 재검토, 무관한 결과 유지 |
| T07 | 게임·부동산 서비스 실행 | 대화·기억·세션·자료·승인이 섞이지 않음 |
| T08 | 서비스 전환·한 서비스 일시 정지 | 다른 실행 유지, 정지 실행의 새 업무만 보류 |
| T09 | 조건 분기 한 경로 미선택 | skipped 경로 때문에 합류가 멈추지 않음 |
| T10 | 실패한 필수 선행 업무 | 후속 시작 차단, 독립 작업은 계속 가능 |
| T11 | 이전 승인·이벤트가 늦게 도착 | 새 시도 진행에 영향 없음 |
| T12 | 도구 승인 거절 | 해당 행위가 실행되지 않고 결과 기록 |
| T13 | 실행 중 초안·공통 역할 편집 | 현재 Run의 그래프·지침 스냅샷 유지 |
| T14 | 두 개발 AI의 병렬 변경 | 별도 worktree, 통합 후 실제 테스트·빌드 |
| T15 | 모델은 완료 주장, 테스트 실패 | succeeded로 표시하지 않음 |
| T16 | 앱 강제 종료·재시작 | 기록 복원, 불확실 실행 중복 수행 없음 |
| T17 | 프로세스 취소 | 하위 프로세스·부분 결과 상태를 확인 |
| T18 | 질문이 서로 순환 또는 무응답 | 제한 후 사용자에게 전달, 무한 대화 없음 |
| T19 | 예산·수정 한도 도달 | 새 업무 보류, 명확한 사용량·결정 표시 |
| T20 | 키 저장소 없음·내보내기 | 키 평문 저장·포함 없음, 세션 입력 가능 |
| T21 | 한글·공백 경로, 설치 패키지 | AI 시작·결과 저장 정상 |
| T22 | 도트 모션 줄이기·키보드 | 목록·텍스트로 모든 필수 기능 수행 |
| T23 | 악성 프로젝트·산출물 ID 참조 | 다른 서비스 데이터 접근 거절 |
| T24 | 두 앱 인스턴스·이중 완료 클릭 | 업무·승인·외부 조치 중복 확정 없음 |

엔진은 Go 테스트와 SQLite 임시 DB로 검증한다. 공급자 이벤트 fixture로 프로토콜 파서·중단·늦은 응답을 확인한다. 화면은 bindings를 대체한 테스트와 실제 native 설치 시험을 함께 사용한다.

실제 모델 검증은 작은 전용 프로젝트와 제한된 예산으로 수행한다. 시험 기록에는 명령·환경·성공/실패·산출물·미검증 사항을 남긴다.

## 15. 플랫폼·패키징·출시

우선 시험 대상: Windows 11 x64, macOS arm64·x64, Ubuntu LTS x64. 최소 OS와 Linux 배포판 범위는 Wails·SDK·CLI·WebView 의존성과 실제 시험 후 확정한다.

- Windows: WebView2, CLI 설치 방식, 공백·한글 경로, 하위 프로세스 종료, 설치 exe.
- macOS: Finder PATH, 두 CPU 아키텍처, Keychain, 실행 권한, dmg/zip, 서명·공증.
- Linux: GTK·WebKit 의존성, 비밀 저장소, X11/Wayland, CLI 권한, deb/압축 배포.
- 공통: 실제 패키지 안에서 SQLite·SDK bridge·CLI 탐지·업무·취소·복구 시험.

각 OS의 CI 또는 실제 시험 장비에서 빌드한다. 로컬 Windows 빌드 성공으로 macOS·Linux 성공을 선언하지 않는다. [Wails 설치 의존성](https://v2.wails.io/docs/gettingstarted/installation/), [CLI](https://v2.wails.io/docs/reference/cli/), [Windows WebView2](https://v2.wails.io/docs/guides/windows/)

초기에는 수동 업데이트로 시작한다. 서명·공증은 실제 자격증명이 준비된 출시 단계에서 수행한다. 자동 업데이트는 DB 이관·백업·실행 중 재시작 문제를 검증한 뒤 추가한다.

완료 정의: R01~R12 구현, T01~T24 증거, 실제 AI 연결 시험, 명시된 세 OS 설치 시험, 알려진 제한·실행 안내. 후속 범위를 임의로 구현하거나 완료를 과장하지 않는다.

## 16. 새 세션에서 이어갈 방법

개발 시작 시 이 명세를 저장소 docs에 복사하고 아래 파일을 만든다. 개발자가 이 문서만 받았다면 먼저 M0부터 시작할 수 있다.

- docs/progress.md: 단계별 상태, 마지막 작업, 다음 작업, 시험 결과, 미검증 항목.
- docs/decisions.md: 기본값에서 변경한 결정과 이유·영향.
- docs/environment.md: Go·Node·Wails·SDK·CLI 버전, OS, 실행·빌드 방법. 비밀정보는 제외.
- docs/acceptance.md: T01~T24 구현·시험 증거와 플랫폼별 결과.

각 기능 단위 완료 또는 작업 중단 전에 다음 형식으로 갱신한다.

    현재 단계:
    구현 완료:
    이번 검증과 결과:
    미검증·알려진 제한:
    마지막 관련 코드/테스트:
    다음에 실행할 구체적인 작업:

새 세션은 이 명세 → progress → decisions → environment → 관련 코드·시험 순서로 읽는다. 대화 기억을 전제로 하지 않는다. 저장소가 있으면 완료된 작업을 반복하지 않고 실제 파일과 시험 결과로 상태를 확인한다.

### 새 개발 세션에 전달할 요청문

> 첨부한 Agent Office 독립 개발 계획서를 기준으로 Go + Wails 앱을 구현해줘. 이전 대화나 메모리는 필요하지 않다. 기존 저장소와 적용 지침을 먼저 확인하고, 구현이 없으면 새 프로젝트를 구성해줘. 문서의 M0~M6 순서로 구현·검증하고, 사용자·AI 협업과 서비스별 데이터 분리를 필수로 지켜줘. 진행 상황과 기술 결정을 docs 파일에 남겨 다음 세션에서도 이어갈 수 있게 해줘. API 키나 다른 OS 시험 환경이 없으면 테스트 provider로 가능한 구현을 계속하되 실제 연결·플랫폼 시험을 완료했다고 표시하지 마.

이 요청문은 개발 착수용이다. 실제 구현에서 적용되는 사용자 지시·저장소 지침·도구 권한을 대체하지 않는다.
