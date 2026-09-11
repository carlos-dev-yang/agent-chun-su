# 외부 서비스 연결 안내

준비된 서비스: Google Drive(Docs·Sheets), Figma, GitHub, Slack, Telegram, Gmail.
현재 Chun-su에 실제 구현된 것은 이 목록 중 **Gmail 읽기 connector**다.
나머지는 설치 매뉴얼과 구현 준비 단계다.

2026-09-11 [별도 실행 점검](../validation/INTEGRATION_RUNTIME_ATTEMPT_2026-09-11.md)에서
대화 진입점이 없음을 확인한 뒤, 사용자 승인으로 `chat`과 설정 처리 프로세스를
구현했다. [현재 실행 검증](../validation/SETUP_CHAT_2026-09-11.md)을 확인한다.

## 춘수와 대화로 설정하기

```sh
bin/chunsu chat
# 또는 첫 요청을 함께 입력
bin/chunsu chat 'Gmail 연결해줘'
```

처음 실행하면 로컬 설정과 DB를 준비하고 별도의 `setup serve` 프로세스를 자동으로
실행한다. 같은 데이터 경로의 worker가 이미 있으면 그 프로세스에 요청한다.
자동으로 시작한 설정 프로세스는 대화 종료 시 함께 종료하며 기존 worker는 유지한다.
별도 데이터 경로는 `--home`으로 선택한다. 실행기 계정 없이도 설정 대화를 시작할 수 있다.

현재는 서비스 이름과 정해진 질문으로 진행하는 **문답형 설정 UI**다. LLM을 호출하지
않으며 일반 업무 자유대화는 아직 지원하지 않는다. 설치 동작은 고정된 호스트 작업이다.

- Gmail: 기존 연결 선택 → 실제 계정 확인으로 재사용. 새 연결은 계정·Desktop client
  JSON 경로·조회 조건 → 범위 확인 → 브라우저 로그인 → 연결 결과까지 진행한다.
- 다른 5개 서비스: 번들 설정 팩을 로컬에 설치하고 해당 매뉴얼을 안내한다. 제공자
  프로그램 자동 설치·계정 연결·Chun-su adapter가 완성된 것으로 표시하지 않는다.
- `취소`/`뒤로`: 현재 단계를 취소하고 서비스 선택으로 돌아간다. 인증 중 취소는
  호스트 정리를 기다린다. `종료`/Ctrl-C/입력 종료는 대화를 끝내고 진행 중 인증을 취소한다.
- 토큰·client JSON 내용은 붙여넣지 않는다. 경로만 받고 OAuth와 Keychain 처리는
  호스트가 담당한다. 메일 수집·AI 공개 승인·스케줄은 연결만으로 활성화되지 않는다.

브라우저를 직접 열려면 `chat --no-browser`를 사용한다. 인증 URL은 로컬 터미널에만
표시하며 기록·공유하지 않는다. OAuth client 준비는 [Google 인증 매뉴얼](../../setup-skills/connect-services/references/google-auth.md)에 있다.

설정 작업 ID로 실패나 중단 상태를 다시 확인할 수 있다.

```sh
bin/chunsu setup status TASK_ID
bin/chunsu setup cancel TASK_ID
bin/chunsu setup stop
```

설정 호스트를 직접 운영하려면 `setup` 후 별도 터미널에서 `setup serve`를 실행한다.
이 프로세스는 AI 작업이나 스케줄을 실행하지 않는다. `setup stop`은 설정 전용
호스트만 종료하며 보고 worker에는 적용되지 않는다. 보고 worker도 사용할 때는
worker를 먼저 실행하고 `chat`을 붙인다. 예전 `gmail connect`/`reauth` 직접 명령은
여전히 단독 제어가 필요하므로, 실행 중 새 연결에는 `chat`을 사용한다.

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
가정하지 않는다. Python 준비 도구와 위 Go 설정 대화의 실행 범위는 다르다.

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
