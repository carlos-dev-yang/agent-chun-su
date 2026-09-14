# PROC-06 — 채팅 운영 매뉴얼 조회

상태: 분리 환경 검증·운영 앞단 반영 완료.

- 공통 `/guide`는 실제 서비스 카탈로그의 6개 안내와 `runtime`을 표시한다.
- `/guide runtime`은 실행 파일에 포함한 구조·복구 순서·명령 예시를 본문으로 표시한다.
  기존 `/guide gmail`도 JSON 래퍼 없이 본문으로 표시됨을 확인했다.
- AI와 컨트롤러가 없는 별도 비공개 데이터 홈에서 위 명령이 동작했고, 조회 후
  컨트롤러 등록·실행 의도·관리 소켓이 생성되지 않았다.
- 임의 경로 형태의 안내 ID, `/install runtime`, 잘못된 인자 수는 거부했다.
  `install_guide`의 runtime 거부는 코드 리뷰로 확인했다.
- 분리 홈에 reception route만 설정하고 실제 Codex 대화를 실행했다. 호스트 추적에
  `read_guide` / `service=runtime`을 확인했고 컨트롤러·워커 구조와 복구 명령을
  한국어로 설명했다. 해당 홈에 컨트롤러 등록이나 enabled 상태는 생성되지 않았다.
- 정확한 패치를 적용한 분리 checkout에서 `conversation`, `reception`, `cli`,
  `telegramchat`의 `go test`가 통과했다. 모두 테스트 소스가 없어 컴파일 확인이다.
  실제 checkout의 해당 패키지 정적 검사와 macOS 빌드도 통과했다.
- 독립 Terra 리뷰의 setup 상태 관측 범위 및 capability 설명 지적을 반영했다.
  새 테스트 소스는 작성하지 않았다. 실제 Telegram DM 발송과 새 플랫폼 메뉴 등록은
  수행하지 않았다. Telegram은 같은 공통 명령 경로와 기존 전송 크기 경계를 사용한다.

## 운영 반영

운영 수신기가 유휴임을 확인한 뒤 검증한 바이너리와 채팅 앞단을 갱신했다.
배포된 실행 파일의 `/guide runtime` 본문을 확인했고, 수신기와 감독자가 살아 있으며
poll이 connected인 상태를 확인했다. 컨트롤러 PID는 반영 전후 동일했다.
컨트롤러·업무 설정·감시 설정은 바꾸지 않았다.

반영 중 기존 `telegram restart`가 시작 응답을 반환한 후 실제 OS 서비스가 내려간
상태를 관측했다. 기존 restart 경로는 stop 뒤 OS 종료 완료를 기다리지 않고 바로
start를 요청한다. 중지 완료와 서비스 부재를 확인한 뒤 `telegram start`로 수신을
복구했다. 해당 재시작 경합의 구현 수정은 이번 읽기 전용 매뉴얼 변경에 포함하지
않았다. 서비스 명령의 성공 응답만으로 준비 완료를 단정하지 않고 `telegram status`의
receiver/supervisor 생존과 poll 상태를 확인해야 한다.
