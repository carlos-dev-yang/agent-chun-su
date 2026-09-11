# 외부 서비스 연결 안내

준비된 서비스: Google Drive(Docs·Sheets), Figma, GitHub, Slack, Telegram, Gmail.
현재 Chun-su에 실제 구현된 것은 이 목록 중 **Gmail 읽기 connector**다.
나머지는 설치 매뉴얼과 구현 준비 단계이며, 아래 준비 도구가 계정을 연결하거나
서비스를 설치하지는 않는다.

2026-09-11 [별도 실행 점검](../validation/INTEGRATION_RUNTIME_ATTEMPT_2026-09-11.md)에서
현재 `worker`는 자연어를 받지 않고 `chat` 진입점도 없음을 확인했다. 아래 AI 안내는
외부 AI에 매뉴얼을 읽히는 방식이다. 춘수 자체와 대화하는 설치 흐름은 아직 없다.
또한 현재 Gmail 연결 명령은 같은 데이터 디렉터리의 worker가 실행 중이면 잠금으로
막힌다. 실제 대화형 설치 검증에는 설정 세션과 호스트 연결 경로 구현이 선행되어야 한다.

## 지금 사용할 수 있는 두 가지 준비 방식

**AI 안내:** 설정을 도울 AI에게 저장소의
[connect-services Skill](../../setup-skills/connect-services/SKILL.md)을 읽히고
“Gmail과 Google Docs를 연결할 준비를 해줘”처럼 요청한다. 선택한 서비스의
매뉴얼만 읽고 기존 연결 여부·목적·허용 범위를 물은 뒤 실제 지원 경로를 안내한다.
이 Skill은 설정 도우미용 자산이며 현재 mail/Jira 보고 실행기에 자동 로드되지 않는다.

**고정 스크립트:** 저장소 루트에서 다음 명령으로 대화형 준비표를 만든다.
Python 3 표준 라이브러리만 사용하는 개발/설정 보조 도구다. Go 배포 바이너리에
Python 의존성을 추가한 것은 아니다.

```sh
python3 setup-skills/connect-services/scripts/prepare.py --interactive
```

계정을 입력하기 전, 서비스·사용 목적·진행 방식을 고르는 단계다. 개별 계정/파일/
채널 범위 질문은 결과의 해당 매뉴얼에서 이어간다. 네트워크 요청, 인증정보 읽기,
패키지 설치, 활성 설정 변경은 하지 않는다. 취소하거나 건너뛸 수 있다.

일괄 준비도 가능하다. `--output`은 새 파일만 만들고 기존 파일은 덮어쓰지 않는다.
출력 경로는 사용자가 선택한다. 비공개 답변·토큰은 문서에 넣지 않는다.

```sh
python3 setup-skills/connect-services/scripts/prepare.py --services all --mode assisted --intent read
python3 setup-skills/connect-services/scripts/prepare.py --services drive,github --mode scripted --intent write
```

`write`는 원하는 사용 목적의 표시다. 전송/수정 권한을 승인하거나 실행하지 않는다.
현재 `chunsu integrations install` 같은 명령은 없으며 이 도구도 해당 명령을
가정하지 않는다. 실제 설치·인증·검증을 수행하는 공통 실행 경로는 다음 구현이다.

## 서비스 매뉴얼

| 서비스 | 처음 사용자에게 묻는 내용 | 매뉴얼 |
|---|---|---|
| Google Drive | Docs/Sheets 중 필요한 것, 계정, 읽을 파일 또는 작성할 위치 | [Drive·Docs·Sheets](../../setup-skills/connect-services/references/drive.md) |
| Figma | 디자인 읽기/생성·편집, 사용 중인 AI 도구, 파일 링크 | [Figma](../../setup-skills/connect-services/references/figma.md) |
| GitHub | 저장소, 읽기/issue·PR 작성, 개인용/조직 배포 | [GitHub](../../setup-skills/connect-services/references/github.md) |
| Slack | 자료 읽기/봇과 대화, workspace·채널, 앱 설치 권한 | [Slack](../../setup-skills/connect-services/references/slack.md) |
| Telegram | 새 봇/기존 봇, 본인 대화/그룹, 응답·알림 목적 | [Telegram](../../setup-skills/connect-services/references/telegram.md) |
| Gmail | 계정, 조회 조건·범위, 읽기/초안·전송 목적 | [Gmail](../../setup-skills/connect-services/references/gmail.md) |

## 최종적으로 제공할 초기 설정 경험

1. “어떤 서비스를 연결할까요?” — 선택하지 않은 서비스는 나중에 연결.
2. 기존 연결이 있으면 재사용 가능 여부를 확인하고 다시 로그인시키지 않기.
3. 서비스마다 한 번에 필요한 질문만 묶어서 제시하고 권한을 평이한 말로 설명.
4. 자동 준비 후 브라우저 로그인 또는 로컬 비밀 입력만 사용자에게 인계.
5. 지정 파일·저장소·채널에서 최소 호출로 접근 범위 확인.
6. “연결됨 / 이 agent에서 사용 가능 / 이 룰셋으로 검증됨”을 구별해서 표시.

로그인 완료만으로 보고서를 읽을 수 있거나 메시지를 보낼 수 있다고 표시하지 않는다.
권한을 넓히지 않는 반복 사용은 재승인을 요구하지 않고, 새 계정·범위·행동에 대해서만
필요한 결정을 요청하는 방향이다. 한 서비스의 자료를 다른 서비스로 보내는 작업은
대상·내용에 대한 권한을 함께 확인한다.

조사 근거는 [공식 사례 조사](../research/INTEGRATIONS_2026-09-10.md),
개발 순서와 미결정 계약은 [구현 준비안](../implementation/INTEGRATION_ONBOARDING.md)에 있다.
