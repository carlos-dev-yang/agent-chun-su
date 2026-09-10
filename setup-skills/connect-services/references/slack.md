# Slack 설치 매뉴얼

확인일: 2026-09-10. 상태: manifest와 외부 설치 경로 준비, Chun-su adapter 미구현.
목표 기능: 봇 멘션/대화, 지정 채널·thread 읽기, 검색, 승인된 답장·알림·댓글.
봇 채널과 사용자의 workspace 자료 검색은 다른 권한 경로다.

## 사용자에게 물을 내용

- Slack 안에서 봇과 대화할 것인가, 기존 채널 자료를 읽고 검색할 것인가, 둘 다인가?
- workspace와 첫 사용할 채널은 무엇인가? 앱 설치 권한이나 관리자 승인이 있는가?
- 본인만 쓸지 팀원도 쓸지? 메시지 전송은 답장만인지 지정 채널 알림도 필요한지?

## 경로 A — 내부용 앱 + Socket Mode

Socket Mode는 공개 HTTP 수신 URL 없이 WebSocket으로 이벤트를 받는다.
앱 manifest로 설정을 준비할 수 있다. 현재 Socket Mode 앱은 공개 Slack
Marketplace에 게시할 수 없으므로 내부 설치용 첫 경로로 권고한다.
[공식 Socket Mode](https://docs.slack.dev/apis/events-api/using-socket-mode/).

1. [Slack 앱 관리](https://api.slack.com/apps)에서 새 앱을 From a manifest로 만든다.
   [최소 멘션 봇 템플릿](../assets/slack-mentions-manifest.json)을 검토해 붙여넣는다.
   템플릿은 멘션 수신·봇 응답·공개 채널 목록에 필요한 권한만 담고 있다.
2. workspace에 설치한다. 관리자 승인이 필요하면 이 단계에서 중단 상태를 보존한다.
3. Basic Information의 App-level token을 `connections:write`로 생성한다.
   bot token과 app-level token은 용도가 다르며 로컬 보안 입력으로 따로 저장한다.
   [app token scope](https://docs.slack.dev/reference/scopes/connections.write/).
4. 사용할 채널에 봇을 초대하고 host에서 channel ID와 허용 사용자 ID를 고정한다.
5. host receiver를 전경으로 실행해 멘션 한 건을 수신하고, 승인된 연결 확인
   답장을 보낸다. 이 receiver는 현재 Chun-su에 없으므로 앱 생성만으로 동작하지 않는다.

템플릿은 완성된 connector가 아니다. 자동으로 앱을 생성하거나 token을 발급하지
않는다. 이름 변경은 가능하며 선택하지 않은 history·DM·search scope는 추가하지 않는다.
[manifest 형식](https://docs.slack.dev/reference/app-manifest/).

## 경로 B — 고정 helper + 읽기/검색 또는 공식 MCP

구현할 helper는 manifest 준비, 비밀 입력, `auth.test`, 채널 확인, Socket Mode
receiver 상태를 각각 처리한다. 검색은 봇 token 하나로 모든 범위를 지원한다고
가정하지 않고 user OAuth 또는 공식 MCP 경로를 별도로 검토한다.

Slack 공식 MCP는 `https://mcp.slack.com/mcp`의 Streamable HTTP와 등록 앱 identity를
사용한다. internal/Marketplace 앱 조건이 있고 DCR은 지원하지 않는다. OAuth와
요청 기능별 scope를 확인하며 일반 사용자 로그인과 bot token을 혼용하지 않는다.
[Slack MCP](https://docs.slack.dev/ai/slack-mcp-server/).

| 추가 기능 | 검토할 권한·설정 |
|---|---|
| 공개/비공개 채널 과거 읽기 | `channels:history` / `groups:history`, 해당 채널 접근/참여 |
| DM 대화 | message.im 이벤트, `im:history`, 앱 메시지 탭; 별도 선택 |
| 메시지 답장 | `chat:write`, host의 목적지/발신자 제한 |
| 사용자 기준 검색 | user OAuth/MCP의 선택된 search scope; private/DM은 필요할 때만 |

Thread 읽기는 `conversations.replies`가 실제 token/채널 유형에서 허용되는지
검증한다. 오래된 공식 문서와 현재 참조의 bot token 설명이 다르므로 “bot token이면
모든 thread를 읽는다”는 약속을 하지 않는다. rate limit도 내부 앱과 외부 배포 조건에
따라 다르므로 해당 설치 유형을 기록하고 Retry-After를 따른다.
[현재 replies](https://docs.slack.dev/reference/methods/conversations.replies/),
[이전 참조](https://api.slack.com/methods/conversations.replies),
[history](https://docs.slack.dev/reference/methods/conversations.history/).

## 첫 검증과 룰셋 사례

- `auth.test`의 team/user/bot identity와 선택한 workspace를 확인한다.
  [auth.test](https://docs.slack.dev/reference/methods/auth.test/).
- 채널 참여, 멘션 이벤트 수신, 허용 thread 읽기, 응답 전송을 따로 판정한다.
- 같은 이벤트 재수신·bot 자신의 메시지·다른 사용자의 멘션이 중복 작업을 만들지
  확인한다. host가 sender/team/channel을 검사하고 쓰기 대상도 다시 제한한다.
- 골든 사례: 인용/forward와 실제 요청자 구별, thread 순서·source 링크 보존,
  비공개 접근 실패를 메시지 부재로 표시하지 않기, 읽은 문장으로 권한 확대하지 않기.
- 메시지 전송 확인은 사용자가 정한 검증용 채널·내용에서만 진행한다.
  이번에는 앱 등록·실제 Slack 조회·메시지 전송을 하지 않았다.

## 문제 해결과 해제

연결됐는데 이벤트가 없으면 Socket 연결뿐 아니라 event subscription, 앱 재설치,
채널 초대를 확인한다. missing_scope는 필요 기능을 다시 확인하고 해당 scope만
요청한다. 조직 검색이 필요하다고 사용자 token을 자동 추가하지 않는다.

해제는 receiver 중지·host 비활성화 후 사용한 app/bot/user token 또는 앱 설치를
철회한다. 기존 메시지는 남긴다. 업데이트 중 다른 채널 봇의 설정을 덮어쓰지 않는다.
