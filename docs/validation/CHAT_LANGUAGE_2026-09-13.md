# CHAT-UX-05/02 언어 설정·영어 시스템 안내 검증

2026-09-13 시작, 2026-09-14 KST 완료.
[승인 목표](../implementation/CHAT_LANGUAGE_GOAL_2026-09-13.md)의 언어 설정과
핵심 시스템 안내 변경을 검증하고 기존 로컬 Telegram 서비스에 반영했다.

## 환경과 변경 범위

- Go + SQLite의 기존 로컬 checkout에서 작업했다. DB 마이그레이션과 설정 전체 재저장은 없다.
- 독립된 private 데이터 홈 두 개를 사용했다. 한 곳은 AI 미설정, 다른 곳은 실제
  Codex CLI 0.153.4 / GPT-5.5 / `native-restricted` 접수 실행기를 사용했다.
- 실제 메일·Jira·업무 실행 권한과 운영 데이터는 검증 홈에 복사하지 않았다.
- `/language`와 `/언어`는 AI 없이 `auto` 또는 정규화한 언어 태그를 저장한다.
  Telegram과 CLI가 같은 처리기·저장 파일·접수 세션을 사용한다.
- 사용자 지침에 따라 새 테스트 소스는 작성하지 않았다.

## 실제 수행한 확인

| 흐름 | 결과 |
|---|---|
| AI 미설정, 파일 없음 | 기본 `auto` 조회와 사용 예 안내 |
| 언어 저장·정규화 | `pt-br` → `pt-BR`, `zh-hant` → `zh-Hant`; 파일 권한 `0600` |
| 잘못된 태그 | 사용 예를 안내하고 직전 설정 유지 |
| 한국어 별칭 | `/언어 현재`, `/언어 초기화` 동작 |
| 재시작·새 대화 | CLI 재시작과 `/reset` 뒤 언어 유지 |
| 말투 메뉴 중 언어 변경 | 언어 명령을 처리하고 메뉴를 닫음; 언어를 말투 선택으로 소비하지 않음 |
| 손상된 언어 JSON | 영어 오류·ID 기록, `/errors` 조회, `/language reset` 복구 |
| private 디렉터리 권한 불일치 | 읽기·저장 실패를 알리고 기존 `zh-Hant` 파일 유지; 권한 복구 후 조회·초기화 성공 |
| `/status`, 수신기 관측 없음 | 컨트롤러 사실은 표시하고 수신·AI·복구 상태는 unknown으로 표시 |
| `/status`, 손상된 수신기 관측 | 컨트롤러 사실은 유지하고 수신기 기록을 읽을 수 없다고 표시 |
| 미실행 `/cancel`·알 수 없는 명령 | 영어 안내, AI 호출 없음 |
| 시스템·말투 UI | 도움말·현재 설정·선택 메뉴·오류는 영어; 저장된 말투 본문은 유지 |

실제 AI 응답도 확인했다. 커스텀 말투에 `Always answer in English. Use one short
sentence.`를 저장한 상태에서 선택 언어 `es`, `ja`, `ko`가 각각 스페인어·일본어·한국어
답변으로 이어졌다. 일본어 생성이 진행되는 동안 `/language ko`를 입력했을 때 설정
완료가 먼저 표시되고 해당 답변은 일본어로 정상 완료됐다. 다음 생성부터 한국어가 적용됐다.

자연어 상태 조회는 `runtime_status` action을 거쳐 한국어 한 문장으로 답했다.
저장된 `action.json`의 `result.detail`은 컨트롤러·수신기 관측과 읽기 가능 여부를
포함한 구조화된 JSON을 유지했다. 고정 `/status`에는 사람이 읽을 영어 문장만 표시됐다.

## 소스 검토와 검사

독립 Terra 리뷰에서 두 가지를 보완했다. 언어 파일뿐 아니라 상위 preferences
디렉터리도 private인지 확인한다. 컨트롤러 응답에서 `active_job`이 누락되거나 null이면
작업이 없다고 단정하지 않는다. 정상 컨트롤러 응답과 권한 불일치 복구는 직접 확인했다.
누락된 IPC 필드, stale 수신기와 Telegram 취소·receipt 경합 경로는 소스 검토로 확인했다.

영향 패키지 검사:

```sh
go test ./internal/chatlanguage ./internal/chatstyle ./internal/conversation ./internal/reception ./internal/errorreport ./internal/telegram ./internal/telegramchat ./internal/cli
go vet ./internal/chatlanguage ./internal/chatstyle ./internal/conversation ./internal/reception ./internal/errorreport ./internal/telegram ./internal/telegramchat ./internal/cli
go build -o <private-validation-home>/chunsu ./cmd/chunsu
git diff --check
```

최종 소스에서 검사 묶음이 모두 통과했다. 해당 패키지에는 기존 test 파일이 없어 `go test` 결과는
컴파일 확인이다. 실제 행동 증거는 위 CLI 흐름과 실제 AI 실행에서 얻었다.

## 최종 언어 확인

자동 모드 검증에서 영어 말투 지침에 영향을 받은 응답이 한 번 관찰되어, 최신 사용자
메시지와 호스트 언어 설정의 우선순위를 더 명확하게 했다. 선호 설정과 단일 언어 규칙을
대화 이력 뒤에 두고, 말투 JSON 다음에 언어 규칙을 전달한다. 명시적 선택은 일반
답변에 적용하고, 요청한 번역·산출물만 해당 언어를 사용할 수 있다.

최종 빌드에서 같은 영어 커스텀 말투로 다음 흐름을 확인했다.

1. `/language en` 뒤 한국어 간식 추천 요청 → 영어 답변.
2. `/language auto` 뒤 같은 한국어 요청 → 한국어 답변. 이전 영어 답변과 말투가
   현재 입력 언어를 덮어쓰지 않았다.
3. `/language ja` 뒤 한국어로 프랑스어 번역 요청 → `Bonjour`. 저장된 언어는 그대로였다.

## 로컬 운영 적용

2026-09-14 08:37 KST에 기존 운영 바이너리를 보존한 뒤 검증 바이너리를 같은 경로에
교체했다. 직전 수신기의 활성 AI 요청과 컨트롤러의 활성 업무가 없음을 확인했다.
기존 수신기·감독자의 종료를 확인한 뒤 등록된 서비스를 다시 시작했다.

- `telegram check`: 봇 식별·페어링·poll 사전 조건 정상, 메시지 소비 없음.
- 새 수신기와 감독자: 둘 다 alive, 등록 의도 enabled, 수신 상태 receiving,
  poll connected, AI 복구 차단 없음, 미보존 오류 0.
- 독립 감시기: process alive와 fresh가 참이고, 재시작 뒤 새 관측에서 컨트롤러 응답,
  poll ready, 관측 완료를 확인했다. 현재·활성 진단은 없었다.
- 운영 홈의 로컬 `/language` → 기본 `auto`와 사용 예. `/status` → 정상 컨트롤러와
  수신기·연결 상태, 활성 AI 요청 없음, 복구 차단 없음의 영어 안내. 이 조회에서 AI를
  호출하거나 운영 언어 설정을 변경하지 않았다.
- 적용 바이너리 SHA-256:
  `e940f5d4b3a23fd80648d8e924998729746e62c7eaa107cfda97206c21873719`.

앞선 CHAT-UX-03 오류·중단 변경도 이번 빌드에 포함해 운영에 반영했다. 실제 Telegram
취소 경합의 실행 증거가 추가됐다는 뜻은 아니다.

## 한계

전체 프로젝트 검사는 실행하지 않았다. 모든 유효 언어 태그의 모델 품질을 검증한 것은
아니며, Telegram 실제 사용자 DM의 언어 명령·취소 경합은 이 검증에서 보내지 않았다.
봇 명령 메뉴 등록·외부 알림 연결은 이번 목표의 범위가 아니다.
