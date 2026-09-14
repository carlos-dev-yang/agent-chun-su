# PROC — 앞단/뒷단 분리 검증

상태: 승인 범위 검증·로컬 반영 완료. 이 문서는 실제 수행한 확인과
미수행 범위를 구분한다. [승인 목표](../implementation/FRONT_BACKEND_BOUNDARY_GOAL_2026-09-14.md)의
PROC-02~05 증거이며 외부 계정·업무 승인 범위를 확대하지 않는다.

## 수행한 검사

실제 checkout에서 `go test`와 `go vet`를 `backend`, `runner`, `service`,
`onboarding`, `cli`, `reception`, `conversation`, `chatsupervisor`, `telegramchat`,
`opsmonitor`에 실행해 통과했다. 이 중 실행되는 기존 테스트가 있는 패키지는
`runner`이며 나머지는 컴파일 확인이다. 새 테스트 소스는 작성하지 않았다.
후속 변경 패키지는 별도 비공개 checkout에서 재확인했고 실제 checkout에서
최종 앞단 정적 검사도 통과했다. macOS 바이너리와 Linux arm64 교차 빌드를 확인했다.
전체 프로젝트 테스트와 네이티브 Linux 서비스 실행은 수행하지 않았다.

Terra 독립 리뷰로 컨트롤러 생명주기, 앞단 명령·취소, 감시 diff를 확인했다.
중지 실패 시 실행 의도 복원, 관측되지 않은 worker intent 표시, 생성 중 고정 명령,
종료 중 async command 결과 처리의 지적을 반영하고 후속 diff를 확인했다.

## 분리된 비공개 데이터 홈에서 확인

실제 사용자 원문·승인·보고서·Telegram binding을 복사하지 않았다. reception/task
route는 해당 호스트의 검증된 Codex 경로로 설정했고 합성 입력만 사용했다.

| 흐름 | 실제 결과 |
|---|---|
| 백엔드 없는 공통 채팅 | `/help`, `/status` 동작, worker 상태는 unknown 표시 |
| 실제 AI 생성과 명령 | 한국어 답변 생성 중 `/status`, 후속 검증의 `/help`와 `/worker status`가 답변을 취소하지 않고 처리됨 |
| task 미설정 컨트롤러 | 공통 `/controller start` 성공, 관리 IPC ready |
| task 미설정 worker | CLI exit 1, `worker_not_configured`, 설정 안내와 오류 ID; 컨트롤러 유지, 새 실행 의도는 disabled |
| 유효한 task route | worker start 성공, 합성 메일 업무 한 건 실행 |
| 진행 중 제어 | controller restart와 worker start 거부, worker stop 성공; 진행 중 업무는 completed로 마무리 |
| 유휴 재시작·중지 | controller restart/stop 성공, 채팅은 계속 명령 처리 |
| 서비스 비정상 종료 | 유휴 검증용 컨트롤러 SIGKILL 후 다른 PID로 OS 복구, 업무 배정 disabled 유지 |
| 명시적 중지 보존 | stop 후 OS에 같은 서비스를 다시 load해도 프로세스가 실행되지 않음; loaded와 running을 분리해 표시 |
| 채팅 복구 | 위 상태에서 같은 채팅의 `/controller start`, `/controller stop` 성공 |
| foreground 소유권 | `controller serve`는 AI 없이 ready, OS start/stop은 해당 foreground를 거부 |
| 기존 setup 소유권 | `setup serve`의 legacy IPC에 managed start를 거부, 해당 프로세스는 유지 |
| 실행 의도 이관 | legacy enabled를 한 번 import; worker stop 후 재시작해도 legacy marker로 다시 켜지지 않음 |
| 의도 파일 손상 | 잘못된 worker intent에도 IPC ready, `worker_intent_unavailable`로 배정 차단 |
| 감시 분리 | chat disabled와 backend degraded/ready를 독립 표시; 설정 실패는 `backend_dispatch_error` |

첫 네이티브 controller restart 검증에서 종료 중 IPC가 `context canceled`를 반환하는
경합을 발견했다. 종료 중 응답 오류를 즉시 실패로 단정하던 경로를 고쳐, 제한 시간 안에
소켓 부재와 writer 잠금 해제를 확인하도록 변경한 뒤 실제 restart/stop이 통과했다.
불확실한 종료를 성공으로 보고하거나 업무를 재실행하지 않는다.

## 로컬 운영 반영

반영 직전 기존 운영 수신기는 connected/유휴, 감독·감시 프로세스는 살아 있었고
컨트롤러는 부재 상태였다. 실행 파일 경로와 유휴 상태를 다시 확인하고 이전
바이너리를 비공개 검증 경로에 보관했다. 기존 감시기·수신기 감독 경로를 중지한 뒤
검증한 바이너리로 교체하고 컨트롤러→채팅→감시기 순서로 시작했다.

2026-09-14 03:12:36 UTC 관측:

- 앞단 `frontend_health=ready`, Telegram receiver·supervisor 생존, 연결 정상.
- 컨트롤러 `available=true`, `controller_ready=true`, OS 관리 서비스로 독립 실행.
- 기존 legacy worker enabled 의도 이관. 운영 task route는 미설정으로 보존하며
  `worker_requested=true`, `worker_running=false`, `worker_error=worker_not_configured`.
- 감시 `backend_health=degraded`, 현재 진단은 `backend_dispatch_error`만 남음.
  이전 `controller_unavailable`은 응답 관측으로 해소되고 관측은 complete.
- 검증용 LaunchAgent는 중지·등록 파일 제거를 마쳤다. 검증 데이터와 이전 바이너리는
  비공개 경로에 남겼고 Git에 포함하지 않았다.

따라서 완료는 앞단 독립과 내부 복구 구조의 구현·반영을 뜻한다. 사용자가 선택하지
않은 운영 task 실행기 설정이나 실제 업무 실행 완료를 뜻하지 않는다.

## 수행하지 않은 확인

실제 Telegram DM 발송, Slack 연결, 외부 알림 발송, 실제 계정 자료로 장애 주입은
수행하지 않았다. Telegram 경로는 소스 리뷰·컴파일과 운영 수신 상태로 확인하며,
공통 채팅 경로의 실제 검증과 구분한다. OS stop 자체 실패/설정 복원 실패를 강제로
발생시키는 검증과 onboarding 인증 중 재시작 거부는 소스 리뷰 범위다.
