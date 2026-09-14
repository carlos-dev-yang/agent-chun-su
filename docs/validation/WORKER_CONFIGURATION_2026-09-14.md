# PROC-09 — 업무용 AI 설정 저장·복원 및 채팅 변경

상태: 구현·검증·운영 반영 완료.

## 변경 계약

- 설정 저장소는 기존 config.json이며 task executor를 복사·모델 변경한다.
- 채팅·CLI 조회는 같은 안전한 필드만 보여 준다. 경로·자격정보·자료 공개 승인
  값은 출력하지 않는다. 원본은 명시적으로 로컬에 등록된 reception/review다.
- 설정 변경은 컨트롤러 IPC와 기존 writer/admission 경계를 사용한다. 워커가
  중지돼 있고 업무·설정 흐름이 유휴인 경우만 허용한다. 저장은 시작을 뜻하지 않는다.
- 실제 실행 설정이 바뀌면 task 자료 공개 승인은 해제한다. 원본 승인 복사나
  기존 reception/review의 유효 설정 변경은 하지 않는다. 동일 설정은 무변경이다.
- 자연어 요청은 조회/설정 변경/단일 restart_worker를 구분한다. 실패나 불확실한
  실행·저장을 자동 재시도하지 않는다. 모델은 사용자가 명시한 값만 요청한다.

## 실제 검증

새 테스트 소스 없이 기존 영향 패키지 검사와 분리 홈의 실제 CLI·IPC를 사용했다.

- 컨트롤러 부재 시 조회는 동작하고 변경은 시작 안내와 함께 거부됐다. 없는 원본과
  잘못된 모델 입력도 설정 파일 변경 없이 거부됐다.
- 원본 복사 후 저장값을 확인했고 워커는 시작되지 않았다. 같은 원본·모델 재선택은
  설정 파일과 승인 상태를 바꾸지 않았다.
- 업무용 모델 변경 시 task 승인·검증 작업 ID가 해제됐고 상속하던 대화/검토 설정은
  기존 값과 승인을 그대로 유지했다. 원본 복사 시에는 Jira 정책 digest도 복사되지
  않았다. 모델만 바꿀 때는 기존 CLI처럼 정책 선택 digest를 유지하되 승인과 검증
  작업 ID가 없으므로 자료 공개 권한으로 사용되지 않는다.
- 실행 중 변경은 `/worker stop` 안내로 거부됐다. 실제 synthetic mail-review 작업이
  실행 중일 때 새 배정을 중지한 뒤 설정을 바꿔도 busy로 거부됐다. 설정은 그대로였고
  기존 synthetic 작업은 completed로 마무리됐다.
- 저장된 설정으로 worker start/restart가 성공했다. 컨트롤러를 종료·재실행하면
  enabled worker가 복원됐다. 명시적 worker stop 뒤 같은 절차에서는 stopped가
  유지됐다. 각 단계에서 설정 파일의 동일성을 확인했다.
- 원본 review 모델을 바꿔도 저장된 task 설정은 바뀌지 않았다.
- 실제 접수 AI가 read_worker_config와 configure_worker를 사용해 선택한 원본을
  저장했다. 보존된 action과 설정 파일에서 성공을 확인했으며 워커는 중지 상태였다.
- 완료 답변이 지침 수락 대신 실제 저장된 driver/model을 설명하도록 안내를
  보완하고 같은 경로를 다시 확인했다. 이어진 자연어 재시작은 restart_worker를
  한 번만 호출했고, stop_worker/start_worker 조합 없이 worker_running이 true가
  됐다. 검증 후 worker stop과 분리 컨트롤러 종료로 정리했다.
- workerconfig, runner, backend, reception, conversation, cli, telegramchat의
  기존 go test/go vet 및 macOS 빌드를 통과했다. runner 외 패키지에는 테스트
  소스가 없어 go test는 컴파일 검사다. runner 테스트의 임시 홈은 macOS 실행
  정책에 맞는 비공개 사용자 디렉터리를 사용했다.

## 실행 환경과 제한

현재 운영 reception의 Codex 0.154.0-alpha.6.2는 접수 역할에서만 검증돼 있다.
같은 설정을 task로 저장하는 것은 가능하지만 기존 task 버전 검사에서 시작이
거부되는 것도 확인했다. 복원 검증에는 공식 OpenAI Codex 릴리스의 0.153.4 macOS
arm64 실행 파일을 분리 홈에만 내려받고 공개된 SHA-256과 일치하는지 확인했다.
기존 버전·모델 검사는 변경하지 않았다. 운영 업무용 실행기·모델을 자동 선택하지
않았으며, 실제 Telegram DM 발송과 Linux 서비스 실동작 검사는 수행하지 않았다.

## 운영 반영

독립 Terra 리뷰의 상태 표시와 실패 응답 지적을 반영했고 최종 차단 사항이 없음을
확인했다. 유휴 운영 환경에 검증한 실행 파일을 반영한 뒤 컨트롤러와 Telegram을
각각 한 번 재시작했다. 컨트롤러는 약 0.69초에 ready, Telegram은 약 30.16초에
새 수신기의 connected/ready를 확인했다. 운영 설정 파일 해시는 반영 전후 동일했고,
worker_requested/running은 false였다. 배포된 CLI의 공통 `/worker config`와
번들 `/guide runtime`에서도 새 설정 흐름을 확인했다. 운영 task 설정은 여전히
미선택이며 이 기능 반영을 업무 실행기 선택·업무 가동 완료로 간주하지 않는다.
