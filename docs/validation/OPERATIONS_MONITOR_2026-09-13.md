# 운영 감시 검증 — 2026-09-13

상태: 주요 흐름 관측·로컬 경보·독립 감시 서비스 구현 및 집중 검증 완료.
현재 Mac의 관측 전용 감시기 등록·실행을 확인했다. 전체 채팅 복구와 외부 알림의
완료를 뜻하지 않는다.

## 범위

[운영 계약](../contracts/operations-monitor-v1.md)에 따라 주요 흐름의 관측,
별도 잠금과 로컬 경보 보존, 독립된 OS 사용자 서비스의 수명주기를 검증한다.
수신기 내부 메시지 분류와 AI 실행 기록을 감시기의 필수 의존성으로 삼지 않는다.

검증 홈은 운영 홈과 분리된 private 디렉터리다. 운영 봇에 메시지를 보내거나
토큰을 읽거나 실제 채팅 프로세스를 종료하는 장애 주입은 수행하지 않는다.
새 테스트 소스는 추가하지 않고 CLI와 기존 관련 검사를 사용한다.

## 수행한 검증

| 검사 | 확인 결과 |
|---|---|
| 읽기 전용 `monitor check/status` | 실행 전후 파일 내용 동일, 미실행/저장/실행 중 상태 구분 |
| `run --once`, foreground 실행과 취소 | 기록 보존, 종료 후 `process_alive=false`, `fresh=false` |
| 중복 실행 | 별도 감시 잠금이 두 번째 실행을 거부, 컨트롤러 잠금 불필요 |
| receipt 표본 | 설정한 개수 이내, 부분 표본 표시, 표본 밖 활성 요청은 ID로 별도 조회 |
| 과거 시각·원문 | 없는 시각을 생성하지 않음, 합성 원문·digest·원시 예외 canary가 출력/경보에 없음 |
| 활성 요청 정체 | 다른 메시지의 최신 시각에 영향받지 않음, 활성 receipt 누락은 복구로 처리하지 않음 |
| poll 진행 | 연결 상태의 진행 정체는 준비 불가/경보, 재시도·진행 시각 불명은 정상화 증거로 사용하지 않음 |
| 경보 변화 | 같은 장애 반복 기록 방지, 각 소스의 정상 증거로만 해제, 보관량 제한 |
| 소스 읽기 실패 | supervisor 파일이 불명확해도 AI·컨트롤러 경보를 기록하고 정상화 가능 |
| 명시적 중지·불명확한 의도 | 중지는 새 채팅 장애 판정을 멈추며 이전 장애를 복구로 기록하지 않음; 잘못된 marker는 별도 경보 |
| 컨트롤러 IPC | endpoint 부재, 오류, `null`, 빈 응답, 잘못된 owner, 불완전한 응답과 정상 응답 구분 |
| 손상된 감시 기록 | 실행을 거부하고 원본 상태 파일 보존 |
| macOS 격리 서비스 | enable·반복 enable·stop·start·remove 완료, 제거 후 기록 보존, 중지 후 managed 실행은 관측하지 않음 |
| 기존 서비스 | 감시기 추가 전후 chat/worker plist 내용 동일(비교 바이너리 경로 제외) |
| 현재 운영 홈 | 읽기 전용 점검과 별도 monitor 등록; 실제 채팅/컨트롤러 종료 없이 함께 실행 |

실행한 코드 검사는 다음과 같다. 새 테스트 소스는 만들지 않았다. 새 감시·CLI·서비스
패키지에는 테스트 파일이 없으며, 기존 runner 테스트와 아래 CLI 검증을 함께 사용했다.

```sh
go test ./internal/opsmonitor ./internal/service ./internal/cli ./internal/chatsupervisor ./internal/runner
go vet ./internal/opsmonitor ./internal/service ./internal/cli
go build -o bin/chunsu ./cmd/chunsu
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o <private-validation-output> ./cmd/chunsu
git diff --check
```

Terra 검토에서 발견한 전역 경보 갱신 조건을 구성요소별 근거로 바꾸고, OS 실행
의도·활성 receipt 시각·컨트롤러 응답 구조 검사를 보완했다. 보완 후 위 집중 검증을
통과했다.

## 현재 Mac 적용

2026-09-13 19:44 KST에 `bin/chunsu monitor enable`로 별도 LaunchAgent를 등록했다.
실제 확인값은 enabled/loaded/process_alive/fresh 모두 true이며, supervisor와 receiver가
살아 있고 poll은 준비 상태, 컨트롤러 probe는 responsive였다. 당시 현재 경보와 미해결
경보는 없었다. 실제 채팅 서비스는 재시작하지 않았다.
19:47 KST 후속 조회에서도 실행·fresh 상태를 유지하고 관측 시각이 계속 갱신됐으며,
현재 경보와 미해결 경보가 없었다.

등록된 실행 파일은 이 checkout의 `bin/chunsu`다. 운영 바이너리 위치를 바꾸면
기존 감시 등록을 제거하고 새 위치에서 다시 등록한다. 관측·경보 확인과 수동 복구는
[운영 안내](../setup/OPERATIONS_MONITOR.md)를 따른다.

## 별도 범위

이메일·macOS 알림·외부 호스트의 신호 누락 감시는 연결하지 않는다.
새 감독자 자동 재시작 정책, 중단 요청의 추가 통지와 재전송도 포함하지 않는다.
기존 채팅 감독자의 수신기 복구는 기존 정책을 따른다.

Linux amd64 빌드는 통과했으나 이번 감시기에 대한 native systemd 실행과 실제 재부팅·
로그아웃 검증은 수행하지 않았다. 실제 봇의 네트워크/AI 장애 주입, 호스트 전원 장애,
일반 AI 답변의 정확한 전송 시각 확장과 중단 요청의 추가 통지도 별도 범위다.
