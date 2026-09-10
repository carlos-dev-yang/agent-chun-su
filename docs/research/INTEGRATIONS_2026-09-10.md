# 6개 서비스 연동과 초기 설정 사례 조사

조사일: 2026-09-10. 공식 제공자 문서와 프로젝트 소스를 우선 확인했다.
온라인 문서의 지원 기능과 Chun-su에서 실제 검증한 기능을 구분한다.
외부 도구를 설치하거나 계정에 로그인한 조사는 아니다.

## 제품 방향

연동 범위는 기본 사용성이고, Chun-su의 차별화 후보는 **특정 룰셋·도구·실행기
조합이 어떤 조건에서 검증됐는지 확인할 수 있는 것**이다. 여기서 증명은
보편적인 정답 보장이나 형식 증명이 아니라 재현 가능한 근거와 적용 범위다.
OpenClaw도 policy-as-code와 provenance를 설명하므로 “경쟁 제품에는 정책이나
검증이 없다”는 주장은 성립하지 않는다. 비교 포인트는 실제 설치된 버전과
권한 범위, 실행 증거, 독립 골든 평가, 후보 채택 기록의 연결 수준이다.
[OpenClaw 정책](https://docs.openclaw.ai/start/why-openclaw/policy-as-code),
[provenance](https://docs.openclaw.ai/start/why-openclaw/provenance).

사용자가 언급한 “grokbot”의 정확한 제품은 미확정이다. 이번 비교는 Hermes와
OpenClaw를 대상으로 했으며, Grokbot과 OpenClaw가 같은 제품이라고 단정하지 않는다.

## 재사용할 사례

| 사례 | 공식 자료에서 확인한 방식 | Chun-su에 적용할 아이디어 |
|---|---|---|
| Hermes 기본 설정 | 터미널과 메시징 gateway, 별도 모델/도구 설정 경로 | 계정 연결과 agent 실행기 연결을 같은 것으로 취급하지 않기 |
| Hermes Google Skill | setup.py로 OAuth 단계를 진행하고 google_api.py가 gws 우선/Python 대체 경로 제공 | 매뉴얼과 결정적 helper를 같은 패키지에 두고 backend 교체 가능하게 만들기 |
| Hermes MCP | 설치 카탈로그, OAuth 로그인, 도구 선택, 재설정. tools/list 성공만으로 인증 완료로 보지 않는 사례 설명 | 실제 허용 리소스 호출을 연결 완료 조건으로 쓰기 |
| Hermes Skill 설정 | 필요한 credential 항목 선언, 로컬 비밀 입력, 메시징에서 로컬 설정으로 인계 | 로그인·토큰 입력만 별도 로컬 화면으로 이동 |
| OpenClaw 온보딩 | 기존 연결 감지, 선택·검증, 건너뛰기·재설정, 채널 설정 wizard | 첫 시작에 6개 전부 강제하지 않고 선택한 서비스만 연결 |
| OpenClaw Skill | SKILL.md와 binary/env 의존성·설치 spec, 로컬/Git/registry 설치 | 서비스 팩마다 필요한 도구·설치법·지원 환경을 명시 |
| Google Workspace CLI | 구조화 출력, auth 설정, 서비스별 Skill과 OpenClaw 설치 메타데이터 | Drive/Docs/Sheets 기능 확장의 CLI 후보 |
| GitHub MCP | 공식 remote/local 서버, toolsets와 read-only 필터 | API 전체를 재구현하기 전에 지원 도구를 좁혀 재사용 |
| Figma | 공식 MCP와 Skill/plugin 설치 안내 | 지원 클라이언트에서는 브라우저 OAuth·파일 링크 중심 UX |
| Slack | manifest로 앱 설정, Socket Mode로 공개 수신 서버 없이 이벤트 연결 | 내부용 첫 설치에 manifest와 채널 선택 제공 |
| Telegram | BotFather로 생성, Bot API polling/webhook | 개인 봇은 polling과 사용자 페어링으로 시작 |

사례 출처:
[Hermes 시작](https://hermes-agent.nousresearch.com/docs/),
[Google Skill 원문](https://raw.githubusercontent.com/NousResearch/hermes-agent/main/skills/productivity/google-workspace/SKILL.md),
[Hermes MCP](https://hermes-agent.nousresearch.com/docs/user-guide/features/mcp),
[Hermes Skills](https://hermes-agent.nousresearch.com/docs/user-guide/features/skills),
[OpenClaw 온보딩](https://docs.openclaw.ai/start/wizard),
[OpenClaw Skills](https://docs.openclaw.ai/tools/skills),
[gws](https://github.com/googleworkspace/cli),
[GitHub MCP 설정](https://github.com/github/github-mcp-server/blob/main/docs/server-configuration.md),
[Figma 설치](https://developers.figma.com/docs/figma-mcp-server/remote-server-installation/),
[Slack Socket Mode](https://docs.slack.dev/apis/events-api/using-socket-mode/),
[Telegram 시작](https://core.telegram.org/bots/tutorial).

이 사례들은 설치 방식의 근거다. 경쟁 제품의 보안·품질을 직접 벤치마크한 결과는
아니다. 외부 매뉴얼의 credential 파일/환경변수 주입 방식을 그대로 채택하지 않고
Chun-su의 호스트 비밀 저장·실행기 격리 조건에 맞춰 검증해야 한다.

## 서비스별 권고 경로

| 서비스 | 개인/내부용 첫 경로 | 확장·대체 경로 | 사용자에게 남는 단계 |
|---|---|---|---|
| Drive — Docs/Sheets | gws를 호스트에서 감싸는 후보; 기존 파일의 URL·탭·범위 지정 | 공식 API 직접 connector; 공식 원격 MCP는 Developer Preview 조건 확인 후 | 최초 OAuth client 준비 또는 배포자가 준비한 앱 로그인, 계정·파일 선택 |
| Figma | Chun-su 자체 읽기는 REST PAT/OAuth; 지원 실행기의 공식 MCP는 편의 경로로 별도 표시 | catalog에 허용된 MCP client 연결 및 검증된 쓰기 경로 | 로그인 또는 로컬 토큰 입력, 파일/노드 선택 |
| GitHub | 제한된 저장소의 fine-grained PAT + 공식 MCP 또는 gh 호스트 wrapper | 배포용 GitHub App; 지원 클라이언트 remote MCP OAuth | 저장소·권한 선택, 조직 승인 필요 시 승인 |
| Slack | 내부 앱 manifest + Socket Mode + 봇 멘션/지정 채널 | 검색·추가 읽기는 user OAuth/공식 MCP; 공개 배포는 HTTP Events 검토 | workspace 설치 권한, 앱 생성·토큰 입력, 채널 초대 |
| Telegram | Bot API long polling + 본인 페어링 | HTTPS webhook 및 그룹/채널 정책 | BotFather 생성, 토큰 로컬 입력, 봇에서 시작 |
| Gmail | 이미 있는 native OAuth/Keychain connector 재사용 | draft/send/modify를 별도 권한으로 확장; gws/공식 MCP는 별도 adapter 후보 | Google 로그인, 계정·조회 범위 선택 |

이는 구현 권고이며 아직 채택된 공통 connector 계약은 아니다. 서비스 지원 목표에는
읽기뿐 아니라 Docs/Sheets 작성, GitHub issue/PR, Figma 생성/편집 가능한 경로,
Slack/Telegram 송수신, Gmail 초안/전송도 포함한다. 설치 한 번으로 모든 작업을
자동 허용한다는 뜻은 아니다.

## 설치 난도를 좌우하는 현실적인 조건

- **Google:** gws는 googleworkspace 조직의 프로젝트지만 README는 공식 지원
  Google 제품이 아니며 변경 가능성이 있음을 명시한다. 바이너리와 사용 API 표면을
  고정해서 검증해야 한다. [gws README](https://github.com/googleworkspace/cli).
  공식 Workspace MCP는 별도 제품이며 조사일 현재 Developer Preview다.
  [공식 MCP 설정](https://developers.google.com/workspace/guides/configure-mcp-servers).
- **Google OAuth:** 사용자별 Cloud project 생성을 영구 기본 UX로 삼으면 설치가
  어렵다. 개인용 BYO client와 배포자 관리 OAuth 앱을 분리해 준비해야 한다.
  External/Testing 앱의 일반적인 refresh token은 7일 만료 조건이 있다.
  [OAuth 만료](https://developers.google.com/identity/protocols/oauth2#expiration).
- **Figma:** 공식 MCP는 catalog에 등재된 client만 접속할 수 있다. Chun-su가
  임의로 직접 등록할 수 있다고 약속하면 안 된다. 범용 생성/편집 도구도 존재하므로
  “공식 MCP 전체는 읽기 전용”이라고 간주하면 안 된다.
  [MCP 소개](https://developers.figma.com/docs/figma-mcp-server/),
  [도구 목록](https://developers.figma.com/docs/figma-mcp-server/tools-and-prompts/).
- **GitHub:** gh의 브라우저 로그인은 편하지만 credential store 실패 시 평문
  저장으로 전환할 수 있다. 이 fallback을 확인 없이 받아들이지 않는다.
  OAuth 편의성과 저장소 단위 권한 제한은 다른 요구다.
  [gh 로그인](https://cli.github.com/manual/gh_auth_login),
  [GitHub App 선택](https://docs.github.com/en/apps/creating-github-apps/about-creating-github-apps/deciding-when-to-build-a-github-app).
- **Slack:** MCP는 등록 앱을 요구하고 internal/Marketplace 앱에 한정된다.
  Socket Mode 앱은 현재 공개 Marketplace에 올릴 수 없으므로 개인 내부 설치와
  공개 배포의 최적 경로가 다르다.
  [Slack MCP](https://docs.slack.dev/ai/slack-mcp-server/),
  [Socket Mode](https://docs.slack.dev/apis/events-api/using-socket-mode/).
- **Telegram:** 봇은 개인 계정 전체 메시지 이력을 읽는 connector가 아니다.
  수신 가능한 메시지는 봇 참여·privacy 설정에 따른다. polling과 webhook을
  동시에 실행하지 않으며 기존 다른 봇 프로세스와 토큰을 공유하는지 확인한다.
  [봇 FAQ](https://core.telegram.org/bots/faq),
  [Bot API](https://core.telegram.org/bots/api#getting-updates).

## 구현 순서 제안

공통 설치 안내/상태 표시와 기존 Gmail을 첫 세로 단위로 묶고, Drive Docs/Sheets와
GitHub, Telegram, Slack, Figma 순으로 실제 adapter를 검증하는 안을 권고한다.
6개 모두 매뉴얼과 기능 목록을 먼저 제공하되 각 연결 화면에 지원 수준을 표시한다.
Telegram은 상시 대화 UX가 우선이면 Drive/GitHub와 순서를 바꿀 수 있다.
Figma REST 읽기와 MCP의 생성/편집 동등성을 약속하지 않는다.

실행 준비는 [연동 안내](../setup/INTEGRATIONS.md), 권한·작업 단위는
[구현 준비안](../implementation/INTEGRATION_ONBOARDING.md)에 정리했다.
