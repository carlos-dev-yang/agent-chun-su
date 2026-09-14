# PROC-08 — 워커 시작 실패의 원인 안내

상태: 수정·분리 환경 검증·운영 앞단 반영 완료.

## 확인한 원인

Telegram 자연어 요청의 보존된 action 기록에서 stop_worker는 processed,
그다음 start_worker는 failed였다. 같은 시점의 접수 오류는 host_failed에
묶였다. 현재 task 실행기는 비어 있고 reception route만 설정되어 있으며,
컨트롤러는 ready, worker_running과 worker_requested는 false,
worker_error는 worker_not_configured다.

backend는 이미 고정된 안전한 원인 문구를 반환하지만, 자연어 접수의 Session.Turn은
오류가 있는 HostResult를 일반 실패로 덮어썼다. FailureMessage도 ActionError를
전부 같은 일반 문구로 표시하여 사용자에게 원인이 전달되지 않았다.

## 수정 경계

워커 제어 경로에서 backend가 생성한 안전한 문구만 명시적으로 전달하고 실패
상태·추적·응답에 보존한다. 임의 오류 원문은 공개하지 않는다. 불확실성·시간 초과
우선순위와 자동 재시도 금지를 유지한다. 업무 실행기·모델·권한·실행 의도를 자동
변경하거나 새 자연어 capability를 추가하지 않는다.

## 수행한 검증

- reception, backend, telegramchat, cli의 go test 및 go vet와 macOS 실행 파일
  빌드를 통과했다. 해당 패키지에는 테스트 소스가 없어 go test는 컴파일 검사다.
- 비공개 분리 홈에 기존 reception 설정만 적용하고 task는 미설정으로 유지했다.
  foreground 컨트롤러를 띄운 뒤 고정 worker restart가 종료 코드 1과 구체적인
  task 실행기 미설정 문구를 반환하는 것을 확인했다.
- 같은 홈에서 실제 Codex에게 자연어로 워커 시작을 요청했다. start_worker가
  한 번 호출됐고 failed로 기록됐으며, 사용자 응답과 action trace 양쪽에 안전한
  task 실행기 미설정 안내가 남았다. generic 내부 작업 실패 문구로 덮어쓰지 않았다.
- 실패 후 컨트롤러는 ready, worker_requested와 worker_running은 false였다.
  분리 컨트롤러는 정상 종료했고 OS 서비스·봇·자격정보는 등록하지 않았다.
- 새 테스트 소스와 실제 Telegram DM 발송 검사는 수행하지 않았다. Telegram과
  로컬 채팅이 공유하는 Session.Turn 및 FailureMessage 경로를 위에서 검증했다.

## 운영 반영과 남은 설정

독립 Terra 리뷰에서 추가 차단 사항이 없음을 확인했다. 운영 수신기의 유휴 상태를
확인하고 검증한 바이너리 반영 뒤 telegram restart를 한 번 실행했다. 약 30.56초
후 ready가 확인됐으며 컨트롤러 PID 9196은 유지됐다. 업무용 실행기 선택은 아직
받지 않았으므로 task 설정과 워커 중지 의도를 변경하지 않았다. 이 변경은 실패
원인 안내 수정이며, 미설정 워커의 실제 실행 복구 완료를 뜻하지 않는다.
