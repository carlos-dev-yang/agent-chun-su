# Telegram 설치 매뉴얼

확인일: 2026-09-10. 상태: 외부 절차 조사 완료, Chun-su receiver/sender 미구현.
목표 기능: 본인과 봇 대화, 선택 그룹의 요청, 승인된 답장·알림·파일 전달.
개인 Telegram 계정의 전체 대화 이력을 수집하는 기능은 Bot API와 다르다.

## 사용자에게 물을 내용

- 새 봇을 만들 것인가, 기존 봇을 쓸 것인가? 기존 봇 token을 사용하는 프로그램이 있는가?
- 본인 DM만 사용할 것인가, 그룹/채널도 사용할 것인가?
- 사용자가 보낸 요청에 답장할 것인가, 별도 일정에 따라 알림도 받을 것인가?

## 경로 A — AI 안내 + BotFather

1. 사용자가 Telegram에서 공식 [BotFather](https://t.me/BotFather)를 열고 `/newbot`으로
   봇을 만든다. 봇 이름과 username을 정한다. 이미 있는 봇이면 재생성하지 않는다.
2. token은 사용자의 로컬 보안 입력 화면에서 저장한다. AI 대화에 붙여넣거나 token이
   들어간 API URL을 브라우저/명령 인자로 만들지 않는다.
3. 사용자가 자신의 봇을 열어 Start 또는 `/start`를 보낸다.
4. host가 받은 실제 sender ID와 chat ID를 로컬 설정에 연결한다. 첫 메시지 발신자를
   무조건 관리자로 등록하지 않고, 로컬에서 시작한 일회용 페어링과 일치시킨다.
5. 사용자가 선택한 대화에 연결 확인 답장 한 건을 보내고 수신을 확인한다.

BotFather와 봇 생성 흐름은 [Telegram 튜토리얼](https://core.telegram.org/bots/tutorial),
봇의 privacy/참여별 수신 범위는 [공식 FAQ](https://core.telegram.org/bots/faq)를 따른다.
페어링을 통한 사용자 ID 입력 최소화는 [Hermes gateway 사례](https://hermes-agent.nousresearch.com/docs/user-guide/messaging)를
참고한 Chun-su 구현 제안이다. 현재 이 단계의 host 기능은 없다.

## 경로 B — 고정 Bot API helper 후보

개인 로컬 설치에서는 long polling을 권고한다. 별도 공개 URL 없이 시작할 수 있다.
helper가 `getMe`로 봇 identity를 확인하고 `getWebhookInfo`로 기존 webhook을
확인한 다음 선택한 수신 경로를 시작한다. 기존 webhook이 있으면 임의로 삭제하지 않는다.

`getUpdates`와 webhook은 서로 배타적이다. update 보존은 최대 24시간이므로
장기 중단 후 모든 요청이 복원된다고 약속하지 않는다. host는 update를 보존하고
중복을 걸러낸 뒤 offset을 진행한다. webhook을 선택하면 HTTPS endpoint와
`secret_token` 검증이 필요하다. [Bot API](https://core.telegram.org/bots/api#getting-updates).

호스트에 둘 범위는 bot identity, sender/chat allowlist, 허용 update 종류,
파일/메시지 크기·예산, 답장/알림 목적지다. token은 일반 OAuth처럼 세부 scope로
나뉘지 않으므로 host가 권한을 제한해야 한다. 그룹 privacy를 끄거나 관리자로
승격하는 작업을 설치 기본값으로 삼지 않는다.

## 첫 검증과 룰셋 사례

- token 유효성, 수신 성공, 본인 페어링, 답장 성공을 별도 판정한다.
- 허용되지 않은 sender/chat에서 실행 요청을 거부한다. display name·username 대신
  실제 ID로 검사하며 전달된 타인의 문장을 관리자 명령으로 처리하지 않는다.
- 중복 update·봇 재시작·기존 polling 충돌·전송 결과 불명확 상황을 확인한다.
  응답을 받지 못했다고 같은 메시지를 무조건 재전송하지 않는다.
- 골든 사례: 대화방을 넘는 자료 유출 방지, 긴 보고서 분할 시 순서와 source 링크
  유지, 포맷 escape, 첨부파일 접근 실패 고지, 원문 속 설치 명령 무시.
- 상시 gateway/일정 알림은 별도 운영 선택이다. 이번에는 봇 생성·수신·전송이나
  background activation을 하지 않았다.

## 문제 해결과 해제

무응답은 Start 여부, bot 차단, privacy/참여, webhook과 polling 충돌을 구별한다.
토큰이 다른 서비스와 공유되면 새 전용 봇을 만드는 대안을 제시하되 기존 token을
자동 교체하지 않는다. API URL 경로에 token이 들어가므로 오류/프록시 로그도 숨긴다.

해제는 receiver 중지와 host 연결/페어링 비활성화 후 필요하면 BotFather에서
token을 철회한다. webhook 제거 시 pending update 삭제를 자동 선택하지 않는다.
사용자의 대화나 로컬 보고서를 자동 삭제하지 않는다.
