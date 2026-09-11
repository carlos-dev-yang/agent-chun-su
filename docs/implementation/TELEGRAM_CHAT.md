# IN-05 — Telegram 개인 대화 테스트

2026-09-11 사용자가 Telegram을 기본 연결로 테스트하고 싶다고 요청했다.
선택한 경로는 **본인 Telegram DM → 로컬 polling 호스트 → 기존 Codex 대화 실행기**다.
공개 webhook 서버 없이 현재 Mac에서 실행한다. 그룹·일정 알림은 이번 기본 연결에 포함하지 않는다.

구현 범위:

- 전용/유휴 BotFather 봇 token을 로컬 비표시 입력으로 받고 기존 Keychain에 저장한다.
- `getMe`와 `getWebhookInfo`로 신원과 기존 webhook 충돌을 먼저 확인한다.
- 로컬 일회용 코드로 본인 private chat과 실제 sender ID를 페어링한다.
- 페어링된 사용자의 텍스트만 AI에 전달하고 같은 DM으로만 답장한다.
- 기존 대화 Skill/스키마와 실행기 격리를 재사용한다. Telegram에서는 상태 조회,
  공개 매뉴얼 읽기·로컬 설치와 작업 목록 조회만 실행한다. Gmail 인증·보고서 본문은
  로컬 CLI로 안내하며 토큰/파일 입력을 원격 채팅으로 요구하지 않는다.
- 수신 update를 중복 처리하지 않고, 처리/전송 결과 불명확 시 자동 재실행하지 않는다.
- `/cancel`, `/reset`, `/help`를 지원한다. 동시에 하나의 요청만 처리한다.

완료 증거는 실제 token 확인 → 본인 페어링 → Telegram 질문 → 실제 Codex 답변 →
허용된 호스트 작업/미지원 안내 → 다른 sender 거부와 중복·취소 확인이다.
토큰·본인 Telegram 입력이 없는 동안 live 연결 성공으로 기록하지 않는다.

공식 근거: [BotFather 튜토리얼](https://core.telegram.org/bots/tutorial),
[Bot API](https://core.telegram.org/bots/api#getupdates),
[FAQ](https://core.telegram.org/bots/faq).
