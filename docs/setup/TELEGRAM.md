# Telegram으로 춘수 운영하기

본인 Telegram DM을 접수창구로 사용한다. 실제 프로그램은 로컬 Mac, Linux 서버,
EC2 또는 회사가 허용한 단말에서 실행한다. 공개 웹훅 서버나 외부 DB는 필요 없다.
채팅 수신, 접수 AI, 업무 실행기는 별도 역할이며 SQLite는 한 컨트롤러만 관리한다.

## 처음 설치하고 계속 실행하기

배포 패키지를 풀고 설치한 뒤 실행한다. 아래 명령은 일반 OS 사용자로 실행한다.

```sh
./install.sh
chunsu telegram enable
chunsu telegram status
```

설치와 연결을 한 번에 진행하려면 `./install.sh --enable-telegram`을 사용한다.
Linux의 비밀 저장소와 systemd 사용자 관리자는 [서버 설치](SERVER.md)의 준비가
필요하다. 회사 비밀 저장소가 없다면 패키지의 선택형 helper를
`./install.sh --with-secret-store`로 설치하고 보호된 키와 저장 경로를 설정한다.

1. 공식 [BotFather](https://t.me/BotFather)의 `/newbot`으로 전용 봇을 만든다.
2. `telegram enable`이 표시하는 로컬 비표시 입력란에 토큰을 입력한다.
   토큰을 Telegram이나 AI 대화에 붙여넣지 않는다. 첫 설치에서만 입력한다.
3. 터미널의 `/start 연결코드`를 자신의 봇 개인 대화로 보낸다. 기본 유효 시간은 5분이다.
4. 연결 확인 후 사용자 서비스가 등록되고 시작된다. `telegram status`에서
   `receiver_alive`, `supervisor_alive`, `receiver.poll`을 확인한다.
   `paired: true`만으로 현재 수신 중이라고 판단하지 않는다.

`enable`은 기본적으로 다음 사용자 로그인 때도 시작한다. Linux에서 로그아웃·재부팅
후 계속 실행하려면 관리자가 승인한 persistent user manager/lingering 구성이 필요하다.
macOS는 로그인한 사용자 세션의 LaunchAgent다. 사용자가 명시적으로 중지하면 다음
로그인에도 중지 상태를 유지한다. `--at-login=false`는 자동 로그인 시작을 설치하지 않는다.

서비스 없이 터미널에서 실행할 때는 `chunsu telegram`을 사용하고, 페어링만 할 때는
`chunsu telegram pair`를 사용한다. 같은 데이터 홈의 수신기를 동시에 두 개 띄우지 않는다.

## 채팅에서 사용할 수 있는 조작

일반 요청은 AI를 부르기 전에 **“접수했습니다.”**를 보낸다. Telegram의 별도 읽음 기능이나
이모지는 사용하지 않는다. 고정 명령은 처리 결과를 바로 답하며 AI가 준비되지 않아도 동작한다.

| 명령 | 의미 |
|---|---|
| `/help` | 사용법 |
| `/status` | 수신·컨트롤러·업무 실행기·큐 상태 |
| `/errors` | 누적 실패, 오류 ID, 횟수와 복구 안내 |
| `/ack 오류ID` | 현재 발생분을 확인 처리; 기록은 보존 |
| `/features` | 설치 가능한 내장 업무 모듈과 안내 자료 |
| `/install workflow-mail-review` | 메일 업무 번들 설치·검증; 기존 활성 기준 보존 |
| `/install guide-gmail` | Gmail 설정 안내 설치; 커넥터 인증 완료와는 구별 |
| `/guide gmail` | Gmail 설정 안내 읽기 |
| `/jobs` | 보존 작업의 ID·상태 |
| `/worker start`, `/worker stop` | 수신기가 소유한 업무 실행기 시작·중지 |
| `/pause`, `/resume` | 업무 큐 일시 중지·재개 |
| `/cancel 작업ID`, `/retry 작업ID` | 지정 작업 취소·기존 규칙에 따른 재시도 |
| `/cancel` | 진행 중 AI 답변의 중단 요청 |
| `/reset` | 실행 정리 후 새 대화 |

업무 실행기는 기본적으로 꺼져 있다. `/worker start` 후 `/status`의 `worker_running`
으로 실제 실행을 확인한다. 기존에 별도로 띄운 worker는 그대로 재사용한다. 수신기가
소유하지 않은 worker는 `/worker stop`으로 종료하지 않으며 `/pause`와 `/cancel`로 업무를
제어할 수 있다. worker 시작은 큐 중지 상태, 자료 반출 승인, 스케줄 활성화를 바꾸지 않는다.
접수한 작업이 `queued`라고 해서 결과가 완성된 것은 아니다.

자연어로도 지원되는 상태 조회·설치·작업 조작을 요청할 수 있다. 기능 ID 밖의 임의
프로그램 다운로드나 shell 실행을 허용하지 않는다. 새 커넥터는 구현과 검증이 필요하다.
계정 인증, 비밀 입력, 보고서 본문 등 로컬 전용 작업은 승인된 로컬/SSH 경로에서 진행한다.

## 실패 확인과 복구

```sh
chunsu telegram status
chunsu telegram check      # 저장된 토큰·봇 신원 확인; 메시지는 소비하지 않음
chunsu errors
chunsu errors show 오류ID
chunsu errors ack 오류ID
chunsu doctor
chunsu telegram restart
```

오류는 단계·분류, 발생 횟수, 최근 시각, 메시지·대화 식별자로 누적한다. 원문,
토큰과 제공자 오류 본문은 저장하지 않는다. 반복 오류는 묶으며 최근 발생 목록은
설정의 `max_messages`로 제한한다. 읽을 수 없는 오류 파일은 보존하고 다른 기록은 표시한다.

통신 단절과 일시적인 polling 실패는 자동 재연결한다. 인증 오류는 비밀 저장소를 다시
읽어 복구를 시도한다. 봇 토큰은 동일한 봇의 것으로 호스트 저장소에서 복구한다.
기존 웹훅이나 다른 프로그램을 자동 삭제·종료하지 않는다. 전송 결과가 불명확한 답장이나
중단된 작업은 중복 실행 위험 때문에 자동 재전송·재실행하지 않는다. `/jobs`로 결과를
확인한 후 필요한 요청만 다시 보낸다. 수신기 재시작 후 대화 맥락은 새로 시작한다.

프로세스 비정상 종료는 감독자가 복구하고, 수신 루프의 heartbeat가 오래 갱신되지 않으면
해당 수신기를 정리한 뒤 다시 시작한다. 프로세스 정체성을 안전하게 확인할 수 없으면
오류를 남기고 확인이 필요한 상태를 유지한다. 컴퓨터 전원·절전, 네트워크, 토큰 철회,
디스크 장애, OS 서비스 관리자 정지는 프로그램만으로 해결할 수 없다. Telegram 자체에
접속할 수 없을 때의 복구 경로는 로컬 또는 SSH다.

```sh
chunsu telegram stop       # 명시적 중지 상태 저장
chunsu telegram start      # 다시 켜기
chunsu telegram remove     # 서비스 등록 제거; 데이터와 오류 기록은 보존
```

업데이트는 같은 위치에 새 바이너리를 설치한 뒤 `chunsu telegram restart`로 적용한다.
실행 파일 경로나 서비스의 저장 환경 경로가 바뀌면 `remove` 후 `enable`로 다시 등록한다.
채팅 오류·수신 메타데이터는 현재 보고서 백업 대상에 포함되지 않으므로 필요한 오류 JSON은
별도로 보관한다. 토큰, 원문, 데이터베이스를 배포 패키지에 넣지 않는다.

AI는 reception route 또는 기본 executor 설정을 사용한다. 초기 Codex 버전/모델 제한은
대화 생성 시 확인한다. 호환성 문제는 `/reset`으로 고쳐지지 않지만 고정 명령은 유지된다.
지원 조합은 [Codex 호환성 기록](../validation/TELEGRAM_CODEX_COMPATIBILITY_2026-09-13.md),
운영 경계는 [채팅 운영 계약](../contracts/chat-operations-v1.md)을 참고한다.
