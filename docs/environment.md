# 개발 환경

비밀정보는 기록하지 않는다.

## 도구 버전 (2026-10-03)

| 도구 | 버전 | 설치 방법 |
|---|---|---|
| OS | macOS 26.4.1 (arm64, Apple M1 Pro) | — |
| Xcode CLT | 2416 | — |
| Go | 1.27.1 | `brew install go` |
| Node.js / npm | 26.10.0 / 11.19.1 | `brew install node` |
| Wails CLI | v2.16.0 | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| Codex CLI | 0.159.2 → 0.160.0 (2026-10-07 확인, ChatGPT 로그인) | 사용자 설치 (`/opt/homebrew/bin/codex`) |
| Claude Code CLI | 2.1.288 (claude.ai 로그인) | 사용자 설치 (`/opt/homebrew/bin/claude`) |
| Claude Agent SDK | 0.3.288 | `runners/claude` npm |

Wails CLI는 `$(go env GOPATH)/bin`에 설치된다. PATH에 추가해야 한다.

## Windows 개발 PC (2026-10-08, M4부터)

| 도구 | 버전 | 비고 |
|---|---|---|
| OS | Windows 11 Pro 10.0.26200 (x64) | — |
| Go | 1.27.1 windows/amd64 | cgo용 gcc 없음 → 이 PC에서는 `-race` 없이 시험 (CI는 race 사용) |
| Node.js / npm | 24.14.0 / 11.9.0 | `wails build`가 실행하는 `npm install`이 lock 파일의 `libc` 항목을 지움 → 커밋하지 않고 되돌림 |
| Git | 2.45.1.windows.1 | 명령 1회 약 0.3초(프로세스 시작 비용) — Git 작업 공간 시험이 느림 |
| Wails CLI | v2.16.0 | 모듈 밖에서 `go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0` (저장소 안에서는 go.mod의 replace 주석 때문에 실패) |
| Codex·Claude CLI | npm 전역 설치 (`%APPDATA%\npm\codex.ps1`·`claude.ps1`) | 이 PC에서 실제 호출은 아직 미시험 |

알려진 차이: 이 PC에는 심볼릭 링크 생성 권한이 없어 `runners/claude`의 심볼릭 링크 시험 1건이 EPERM으로 실패한다(CI Windows에서는 통과).

## 실행·빌드

    wails doctor
    go test ./...
    wails dev
    wails build

## 미확인 환경

- Ubuntu LTS x64, macOS x64: 시험 장비 없음 (CI 빌드·시험만)

## 프론트엔드 명령 (frontend/)

`main.go`가 `frontend/dist`를 embed하므로 새로 받은 저장소에서는 `go test`/`go build` 전에 `npm run build`를 한 번 실행한다.

    npm test          # Vitest (jsdom)
    npm run typecheck
    npm run build

## 알려진 경고

- `wails dev`/`build` 링크 시 `built for newer 'macOS' version (13.0) than being linked (11.0)` 경고. Go 1.27 최소 macOS(13)와 Wails 기본 배포 대상(11) 차이로 보이며, 출시 전 최소 macOS 버전을 정할 때 함께 정리한다.
- npm 11의 install-scripts 승인 기능 때문에 esbuild·fsevents 설치 스크립트가 실행되지 않았지만 빌드·테스트에는 영향이 없었다.

## 데이터 위치

- 기본: `os.UserConfigDir()/AgentOffice/agent-office.db` (macOS: `~/Library/Application Support/AgentOffice`)
- `AGENT_OFFICE_DATA_DIR` 환경변수로 바꿀 수 있다 (시험용).
- 실제 OS 키체인 시험: `AGENT_OFFICE_KEYRING_IT=1 go test ./internal/secrets/ -run TestRealKeychain`

## Claude bridge (runners/claude)

    npm install
    npm run build      # dist/main.js 생성
    npm test

## 실제 공급자 시험 (사용자 계정·사용량 사용, 기본 비활성)

    AGENT_OFFICE_CODEX_IT=$(which codex) go test ./internal/providers/ -run TestCodexReal -v
    AGENT_OFFICE_CLAUDE_IT=1 go test ./internal/providers/ -run TestClaudeReal -v

## 실제 M4 게이트 (사용량 사용, 기본 비활성, 약 25분)

    AGENT_OFFICE_REAL_GATE=<제어 폴더> AGENT_OFFICE_CODEX_IT=<codex 실행 파일> go test ./internal/engine/ -run TestRealM4Gate -v -timeout 120m

사람이 결정할 항목은 `<제어 폴더>/pending/<id>.json`에 나오고, `<제어 폴더>/decisions/<id>.txt`에 결정을 쓰면 반영된다(형식은 시험 코드 주석). Windows에서 Codex 실행 파일은 `%APPDATA%\npm\codex.cmd`.

## 실제 공급자 취소 시험 (사용량 사용, 기본 비활성)

    AGENT_OFFICE_CODEX_IT=$(which codex) go test ./internal/providers/ -run TestCodexRealCancel -v
    AGENT_OFFICE_CLAUDE_IT=1 go test ./internal/providers/ -run TestClaudeRealCancel -v
