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

Wails CLI는 `$(go env GOPATH)/bin`에 설치된다. PATH에 추가해야 한다.

## 실행·빌드

    wails doctor
    go test ./...
    wails dev
    wails build

## 미확인 환경

- Windows 11 x64, Ubuntu LTS x64, macOS x64: 시험 장비 없음

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
