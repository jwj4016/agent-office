# Agent Office 설치·실행 안내

Agent Office는 AI(Claude·Codex)와 사람이 같은 업무 흐름에서 일하는 데스크톱 앱입니다. 이 문서는 설치, 첫 실행, 데이터 위치, 문제 확인 방법과 알려진 제한을 정리합니다.

## 지원 범위

| OS | 패키지 | 시험한 환경 |
|---|---|---|
| Windows 11 x64 | `AgentOffice-<버전>-windows-x64-setup.exe` | CI(windows-latest) 설치·진단·제거, Windows 11 Pro 개발 PC(M4 게이트) |
| macOS 13 이상, Apple Silicon | `AgentOffice-<버전>-macos-arm64.dmg` | CI(macos-latest) 설치·진단, macOS 26 개발 Mac |
| macOS 13 이상, Intel | `AgentOffice-<버전>-macos-amd64.dmg` | CI(macos-15-intel) 설치·진단 |
| Ubuntu 24.04 LTS x64 | `AgentOffice-<버전>-linux-amd64.deb` | CI(ubuntu-24.04) 설치·진단·제거 |

표에 없는 환경(Windows ARM64, 다른 Linux 배포판, macOS 12 이하)은 시험하지 않았습니다. macOS 최소 버전이 13인 이유는 포함된 Claude 실행 파일이 macOS 13 이상을 요구하기 때문입니다.

## 미리 필요한 것

| 쓰려는 기능 | 필요한 것 |
|---|---|
| Claude 연결 | Node.js 18 이상, Anthropic API 키(기본). 개인 사용이라면 이 PC의 Claude 로그인을 직접 선택할 수 있습니다 |
| Codex 연결 | Codex CLI 설치와 로그인(`codex login`) |
| 코드 업무를 병렬로 실행 | Git. 없으면 코드 업무를 하나씩 공유 폴더에서 실행합니다 |
| OpenAI·Claude 모델 API 연결 | 각 API 키 |

Claude 실행 프로그램(Claude Agent SDK와 OS별 Claude 실행 파일)은 패키지에 들어 있습니다. Node.js는 포함하지 않습니다.

## 설치

### Windows

1. `…-windows-x64-setup.exe`를 실행합니다. 관리자 권한을 요청합니다.
2. 서명되지 않은 설치 파일이라 SmartScreen이 "Windows의 PC 보호" 창을 띄웁니다. **추가 정보 → 실행**을 누릅니다.
3. 기본 설치 위치는 `C:\Program Files\Agent Office\Agent Office`입니다. 한글·공백이 들어간 폴더에도 설치할 수 있습니다.
4. WebView2 런타임이 없으면 설치 프로그램이 내려받아 설치합니다(인터넷 연결 필요).

제거: 설정 → 앱 → Agent Office → 제거. 사용자 데이터는 지우지 않습니다.

### macOS

1. dmg를 열고 **Agent Office**를 **Applications**로 끌어 놓습니다. Mac의 CPU에 맞는 dmg를 받아야 합니다. Apple Silicon은 arm64, Intel은 amd64입니다.
2. 공증되지 않은 앱이라 처음 열 때 "확인할 수 없음" 경고가 뜹니다. **시스템 설정 → 개인정보 보호 및 보안**에서 **그래도 열기**를 누릅니다(한 번만).
3. CPU에 맞지 않는 dmg를 설치하면 연결 확인과 진단에 "이 OS(darwin-arm64)용 Claude 실행 파일이 없습니다"가 표시됩니다.

Finder에서 연 앱은 PATH가 짧아서, Homebrew·npm으로 설치한 `node`·`codex`는 `/opt/homebrew/bin`·`/usr/local/bin`·`~/.npm-global/bin`·`~/.volta/bin`에서 찾습니다. 다른 위치라면 연결 설정에서 실행 파일 경로를 지정합니다.

### Ubuntu 24.04

    sudo apt install ./AgentOffice-<버전>-linux-amd64.deb

필요한 GTK·WebKitGTK 라이브러리는 apt가 함께 설치합니다. 앱은 `/usr/lib/agent-office`에, 실행 명령은 `/usr/bin/agent-office`에 설치되며, 프로그램 메뉴에도 추가됩니다. 제거는 `sudo apt remove agent-office`입니다.

API 키를 OS 키 저장소에 보관하려면 Secret Service(GNOME 키링 등)가 실행 중이어야 합니다. 없으면 키는 앱을 끌 때까지만 메모리에 있고, 설정 화면에 그렇게 표시됩니다.

## 데이터 위치

| OS | 위치 |
|---|---|
| Windows | `%AppData%\AgentOffice` |
| macOS | `~/Library/Application Support/AgentOffice` |
| Linux | `~/.config/AgentOffice` |

API 키는 이 폴더가 아니라 OS 키 저장소(Windows 자격 증명 관리자, macOS 키체인, Linux Secret Service)에 `agent-office` 이름으로 보관됩니다. 앱을 제거해도 데이터와 키는 남습니다. 서비스를 다른 곳으로 옮길 때는 서비스 개요의 **내보내기 (키 제외)**를 씁니다.

데이터 폴더 하나는 앱 하나만 열 수 있습니다. 앱을 두 번 실행하면 이미 열린 창이 앞으로 옵니다.

## 업데이트

자동 업데이트는 아직 없습니다. 앱을 닫고 새 패키지를 같은 방법으로 설치합니다. 데이터 폴더는 그대로 쓰이며, 저장소 형식이 바뀌면 시작할 때 자동으로 변환됩니다. 실행 중이던 업무는 다음 시작 때 '중단됨'으로 표시되고, 실행 화면에서 다시 시도할 수 있습니다.

## 문제 확인 (진단)

설치된 앱이 스스로 점검합니다. 창은 열리지 않고, 새 임시 폴더에서만 시험하므로 사용자 데이터는 바뀌지 않습니다. API 호출도 하지 않습니다.

    # macOS
    "/Applications/Agent Office.app/Contents/MacOS/agent-office" --self-check
    # Linux
    agent-office --self-check
    # Windows (PowerShell) — 결과는 파일로 저장됨
    & "C:\Program Files\Agent Office\Agent Office\agent-office.exe" --self-check -out "$HOME\Desktop\agent-office-진단.json"

점검 항목은 다음과 같습니다.

- 데이터 저장과 다시 열기
- 키 저장소
- 포함된 Claude 실행 프로그램(SDK 로드와 Claude 실행 파일 시작)
- Codex·Git 탐지
- 테스트 AI로 서비스 개발 흐름을 끝까지 실행
- 실행 취소
- 앱 재시작 뒤 기록 복원

`[failed]`는 앱이 제대로 동작하지 않는다는 뜻이고, `[warn]`은 그 기능을 쓸 때만 필요한 것이 없다는 뜻입니다.

실제 AI 연결은 앱의 **설정 → 연결**에서 확인합니다. 실행 파일, 버전, 인증, 실제 호출을 단계별로 따로 보여 줍니다.

## 알려진 제한

- **서명·공증 없음**: macOS는 ad-hoc 서명만 되어 있고, Windows 설치 파일은 서명되지 않았습니다. 첫 실행 때 OS 경고를 직접 허용해야 합니다.
- **자동 업데이트 없음**: 새 버전은 직접 내려받아 설치합니다.
- **Node.js는 별도 설치**: Claude 연결에만 필요합니다.
- **Claude 로그인 방식**: 배포 기본값은 API 키입니다. 이 PC의 Claude 로그인은 개인 사용을 위한 선택 항목입니다.
- **Codex 연결**: codex-cli 0.159~0.161에서 시험했습니다. CLI가 크게 바뀌면 연결 확인에서 실패로 보일 수 있습니다. 중단된 Codex 대화는 이어 가지 않고 새로 시작합니다.
- **모델 API 연결(OpenAI·Claude API)**: 텍스트·JSON 결과만 만들 수 있고 파일·코드 변경 결과는 만들 수 없습니다. 실제 API 키로는 아직 시험하지 않았습니다.
- **Git 작업**: 서비스 작업 폴더가 Git 저장소이면 `agent-office/…` 브랜치와 worktree가 생깁니다. 사용자 브랜치에는 직접 병합하지 않으며, 반영은 사람이 합니다. 개발자 도구가 없는 Mac에서는 `/usr/bin/git`을 쓰지 않고 Git이 없는 것으로 처리합니다.
- **사무실 화면**: 회의 탁자가 하나라서, 동시에 열린 회의가 여럿이면 나머지는 글로만 표시합니다.
- **창 하나**: 여러 창이나 여러 데이터 폴더를 동시에 열 수 없습니다.
- **Linux 화면**: CI의 가상 디스플레이(xvfb)에서만 실행했습니다. 실제 X11·Wayland 데스크톱과 GPU별 동작은 시험하지 않았습니다. WebKitGTK는 일부 GPU 드라이버에서 빈 창이 보이는 문제가 알려져 있으며, 그럴 때 `WEBKIT_DISABLE_DMABUF_RENDERER=1 agent-office`로 실행해 볼 수 있습니다(이 앱에서 재현해 본 것은 아님).
- **성능 수치**: `docs/acceptance.md`의 측정값은 정해진 시험 시나리오 결과이며, 성능을 보장하는 값이 아닙니다.
