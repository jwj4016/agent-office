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
