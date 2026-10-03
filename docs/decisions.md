# 기술 결정 기록

명세 1장의 기본값과 다르게 정하거나, 명세가 열어둔 항목을 확정한 내용을 기록한다.

| 날짜 | 결정 | 이유 | 영향 |
|---|---|---|---|
| 2026-10-03 | 체크리스트는 `docs/progress.md`에서 관리 | 명세 16장의 진행 파일과 통합해 파일 수를 줄임 | 새 세션은 progress.md 하나로 상태 확인 |
| 2026-10-03 | 개발용 Node는 Homebrew 기본(26.x) 사용 | 설치 시점의 Homebrew 기본 버전 | 출시 전 지원 Node 범위를 별도로 확정 |
| 2026-10-03 | 테스트 provider는 `StartRequest.Model`을 시나리오 이름으로 사용 | 별도 필드 없이 fixture 선택 | 실제 연결과 구분되도록 provider 이름은 `test` |
| 2026-10-03 | Codex 연동은 codex-cli 0.159.2의 `app-server generate-ts` 스키마 기준. `ephemeral: true` 스레드, 승인 정책 `on-request`/`never`, 기본 sandbox `read-only` | 설치 버전 스키마로 확인. 재개(Resume)는 미지원으로 표시 | CLI 업데이트 시 스키마 재확인 필요. 모델링하지 않은 서버 요청은 JSON-RPC 오류로 거절 |
| 2026-10-03 | Claude bridge는 `@anthropic-ai/claude-agent-sdk` 0.3.288, `settingSources: []`로 사용자 Claude Code 설정을 읽지 않음 | 사용자 설정의 넓은 권한이 앱 정책을 우회하지 않도록 격리 | CLAUDE.md 등 프로젝트 설정도 로드되지 않음. 필요 시 정책으로 명시 허용 |
| 2026-10-03 | 하위 프로세스는 Unix에서 별도 프로세스 그룹으로 실행하고 취소 시 그룹 전체 종료, Windows는 `taskkill /T` | 테스트 러너 등 손자 프로세스까지 정리 | Windows는 M6에서 Job Object 검토 |
| 2026-10-03 | 업무 시도 상태에 `superseded` 추가 | 재작업으로 대체된 시도를 실패·취소와 구분해 기록 보존 | 화면에서 "대체됨"으로 표시 |
| 2026-10-03 | 스트리밍 message_delta는 DB에 저장하지 않고 화면에만 전달, 완성된 message는 저장 | 저장량을 줄이고 DB를 상태 기준으로 유지 | 재시작 후에는 완성된 메시지만 보임 |
| 2026-10-03 | AI 산출물은 엔진이 지정한 시도별 폴더(`<data>/projects/<id>/runs/<run>/attempts/<att>/out`)에 파일로 쓰게 하고, 단일 텍스트 결과만 최종 응답으로 대체 허용 | 완료 판정을 실제 파일로 하기 위함 | M2 실제 공급자의 쓰기 권한(작업 폴더 밖) 설정 필요 |
| 2026-10-03 | report 결과는 JSON 객체 | 리뷰 결과를 분기·후속 입력으로 구조적으로 쓰기 위함 | AI report도 JSON으로 작성 지시 |
