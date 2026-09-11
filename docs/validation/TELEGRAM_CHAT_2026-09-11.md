# IN-05 — Telegram 기본 대화 연결 검증

날짜: 2026-09-11, macOS/arm64. 기준: `35daf66`의 실제 AI 터미널 대화 이후.
사용자가 기본 연결로 Telegram 테스트를 요청했다. 실제 봇 token과 본인 페어링은
아직 제공되지 않았으므로 **live Telegram 연결 성공은 미검증**이다.

## 구현한 범위

`bin/chunsu telegram` 한 명령으로 로컬 token 입력 → Keychain 저장 → 일회용 코드로
본인 DM 페어링 → 수신·AI 대화·같은 DM 답장까지 진행한다. 다음 실행은 저장된 연결을
재사용한다. `/help`, `/cancel`, `/reset`을 제공한다. 공개 webhook 서버는 필요 없다.

AI 실행기는 기존 Codex와 `converse` Skill을 사용한다. 대화 채널의 capability 목록은
기존 목록에서 상태·작업 목록·공개 매뉴얼 읽기/설치만 선택한다. Gmail 인증과 보고서
본문을 원격으로 노출하지 않는다. 기존 보고 실행기·DB schema·출시 게이트는 변경하지 않았다.

설정 내용, 수신 claim/cursor, 전송 결과를 private 파일에 보존한다. token은 Keychain,
대화 원문은 메모리에만 둔다. 다른 발신자·그룹·forwarded/via-bot 입력은 거부한다.
중복 claim과 불명확한 전송은 자동 재실행하지 않는다. 프로세스는 foreground로 실행한다.

## 실제 수행한 검사

Bot API 통신은 명시적 loopback 모의 HTTP 서버와 합성 token/사용자 ID를 사용했다.
AI는 가짜 응답이 아니라 기존 로그인으로 실행한 **Codex CLI 0.153.4 / gpt-5.5**다.
검사 전용 홈을 사용했고 검사 후 합성 token의 Keychain 항목과 token 파일을 제거했다.

| 시나리오 | 관찰 |
|---|---|
| token 파일 → getMe/webhook 확인 → Keychain → 일회용 code | 신원 확인, 1:1 페어링, 확인 답장의 receipt 보존 |
| “회의 시작 인사 한 문장” → “방금 문장을 더 짧게” | 실제 모델 답변과 대화 맥락 유지, 같은 페어링 DM으로 전달 |
| 설정 자료 설치 후 실행 상태 확인 | 실제 `install_guide` → `runtime_status`, 호스트 결과를 사용하는 후속 답변 |
| 페어링되지 않은 sender | AI 호출·답장 없음 |
| 이미 처리된 update ID 재수신 | 추가 답장·호스트 실행 없음 |
| “Gmail 로그인을 여기서 끝내고 client JSON을 붙여넣을까” | JSON/비밀을 받지 않고 Mac의 `chunsu chat`으로 인계하도록 안내 |
| AI가 생성 중인 설치 요청에 `/cancel` | executor 중단, receipt `cancelled`, 취소 확인 답장 |
| `/reset` 및 SIGINT 종료 | 맥락 초기화 답장, 프로세스 정상 종료 |
| race detector 빌드로 위 전체 경로 재실행 | 실제 AI와 모의 Telegram 경로 통과, race 진단 없음 |
| 가상 터미널에서 합성 token 비표시 입력 | ECHO 꺼짐, token이 출력에 없음, 입력 후 ECHO 복구 확인 |
| 페어링 code 만료 | 연결되지 않은 상태로 시간 제한에 종료 |
| 기존 webhook 응답 | 거부, binding 미발행, webhook 삭제 요청 없음 |
| 페어링 확인 답장을 서버가 수신 후 오류 반환 | `pair_reply_unconfirmed`, 한 번만 전송 시도 |
| 위 불명확 상태에서 프로세스 재시작 | 저장된 본인 binding 재사용, 확인 답장 자동 재전송 없음 |

오류 모의 서버는 마지막 SIGINT로 끊긴 polling 응답에서 connection-reset 진단을
출력했다. 춘수의 정상 종료로 요청 연결이 닫힌 것이며 검증 assertion 실패나
live 제공자 장애로 기록하지 않았다.

## 빌드·기존 검사

- 일반 Go 빌드 및 race detector 빌드 성공.
- `go test ./internal/runner ./internal/feedback ./internal/secrets`: runner/feedback 기존
  테스트 통과; secrets 패키지는 기존 테스트 파일 없음.
- `go vet ./internal/telegram ./internal/cli ./internal/conversation ./internal/platform` 통과.
- 최종 바이너리의 기존 터미널 `chat`에서 실제 Codex 응답 확인; Telegram 전용
  안내로 강제되지 않고 로컬 회의 준비 대화가 유지됐다.
- 변경 문서 로컬 링크 91개와 `git diff --check` 확인. 검사 홈의 JSON 파일에서
  합성 token 문자열이 보존되지 않았음을 검사했다.
- 사용자 홈의 `telegram status`는 `configured: false`. live 연결은 아직 시작하지 않았다.
- 새 영구 테스트 파일은 추가하지 않았다. 직접 실행한 모의 서버/가상 터미널 검사 도구와
  합성 runtime 데이터는 Git 제외 경로에 두고 저장소에는 결과만 남겼다.

## 남은 검증·한계

- 실제 BotFather token, Telegram 사용자 DM, 실제 getMe/polling/sendMessage 수신·답장 확인.
  사용자 봇 선택과 로컬 token 입력 이후 진행한다. 모의 API 통과를 대신 증거로 삼지 않는다.
- 실제 제공자 rate limit/장시간 네트워크 단절, 두 외부 polling 프로그램의 충돌,
  긴 Unicode 답장 전체 경계, 파일/그룹 기능은 이번 live 검증에 포함되지 않는다.
- `list_jobs`는 터미널에서 검증한 동일 호스트 함수를 재사용한다. Telegram에서 실제 개인
  작업 metadata를 전송하는 별도 시나리오는 실행하지 않았다.
- 본인 재페어링/토큰 교체 UI, 장기 대화 보존, receipt 정리·복원, 강제 종료 뒤 자동 복구,
  백그라운드 자동 시작/일정 알림은 구현하지 않았다. 한 번에 한 요청만 처리한다.
- 전체 프로젝트 테스트·보고 실행기의 새 모델 재검증·평가기 격리/골든 품질 게이트는
  이번 작업 범위가 아니다. 원문 비밀 격리와 의미적 정확성은 별도 판단이다.

사용법: [Telegram 연결](../setup/TELEGRAM.md).
계약: [Telegram private chat v1](../contracts/telegram-v1.md).
