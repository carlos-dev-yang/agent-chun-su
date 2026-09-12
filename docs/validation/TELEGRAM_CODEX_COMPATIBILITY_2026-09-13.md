# IN-05 / MOD-03 — Telegram Codex 업데이트 호환성 복구

날짜: 2026-09-13, macOS/arm64. 사용자가 `bin/chunsu telegram` 실행 중
터미널의 버전 검증 오류와 Telegram의 일반 실패 답장을 보고하고 수정을 요청했다.

## 원인과 변경

실제 대화 설정은 ChatGPT 앱에 포함된 Codex를 가리켰으며 `--version` 결과는
`codex-cli 0.154.0-alpha.6.2`였다. 대화 실행기는 `codex-cli 0.153.4`와의
문자열 일치만 허용하여 모델 호출 전에 요청을 거부했다. Telegram 전송·긴 대화가
원인인 것은 아니며, 일반 오류에 붙인 `/reset` 안내는 이 문제에 적용되지 않았다.

- macOS/arm64의 reception 경로에 새 버전과 기존 `gpt-5.5` 조합을 추가했다.
  기존 `0.153.4`도 유지한다. 역할별 검사를 실행기와 `doctor`가 공유한다.
- Telegram 시작 시 실제 reception 설정을 검사하여 미지원 상태에서 수신 update를
  소비하지 않는다. 명시적 reception 설정도 사용한다. 매 생성의 재검사는 유지한다.
- 실행 중 호환성이 바뀌면 Telegram에도 실제 버전/지원 범위와 설정·업데이트 안내를
  전달한다. 일반 오류에서 무조건 제시하던 `/reset` 문구를 제거했다.
- 파일 읽기 범위, 쓰기·네트워크 제한, native tool/MCP/Skill 탐색 차단,
  구조화된 호스트 작업 계약과 사용자 설정·페어링·자격 증명은 변경하지 않았다.
- task와 review는 계속 `0.153.4 / gpt-5.5`를 요구한다. 새 reception 검증을
  원문 자료를 사용하는 보고서·평가 실행의 승인으로 확장하지 않았다.

## 실제 수행한 검사

기존 `var/telegram-check/walkthrough.py`를 수정본으로 실행했다. Telegram은
명시적 loopback 모의 Bot API와 합성 token/사용자를 사용했고 AI는 실제 설치된
Codex `0.154.0-alpha.6.2 / gpt-5.5`, 기존 CLI 로그인을 사용했다.
검사 홈은 `var/telegram-djl8eeyn`이며 Git 제외 대상이다.

| 검사 | 관찰 |
|---|---|
| 봇 신원 확인·일회용 코드 페어링 | 모의 API를 통해 성공, 확인 답장 저장 |
| 회의 인사 요청 → 더 짧게 수정 | 실제 AI 답변 2회, 앞선 문장의 맥락 유지 |
| Telegram 설정 자료 설치 → 상태 확인 | 실제 `install_guide`와 `runtime_status`, 후속 AI 답변까지 완료 |
| 타 사용자 입력 / 같은 update 재수신 | 답장 없음 / 추가 실행 없음 |
| Telegram에서 Gmail 인증·client JSON 입력 요청 | 로컬 인증 경로로 안내, 원격 인증 실행 없음 |
| `/cancel`, `/reset`, SIGINT | 생성 중단·초기화·정상 종료 |
| 보존된 실행 metadata | 생성 6회 exit 0, 취소 1회 interrupted, native tool 관찰 없음 |
| 명시적 reception 경로를 쓰는 실제 CLI 대화 | shell 파일 생성 요청을 미지원으로 안내, 생성 파일 없음 |
| 역할별 `doctor` | 사용자 설정의 reception은 prerequisites_match, task/review는 needs_revalidation |
| 잘못된 실행 파일을 선택한 Telegram 시작 | 실제/지원 버전 출력 후 exit 1, token 입력·polling 전에 거부, Telegram 상태 디렉터리 없음 |

기존 검사 도구가 합성 token의 Keychain 항목과 token 파일을 정리하고 exit 0으로
종료했다. 사용자 Telegram token과 DM은 이 검사에 사용하지 않았다.

별도 `var/telegram-boundary-g_gnj54_`의 합성 marker와 실제 Codex sandbox에
앱과 같은 named permission profile을 적용했다. 패키지 파일 읽기는 exit 0,
패키지 밖 private 파일 읽기와 패키지 안/밖 쓰기는 `Operation not permitted`,
쓰기 marker는 없었다. 동적 loopback HTTP 주소는 sandbox 밖에서 exit 0과 `OK`,
안에서는 curl exit 7로 연결이 차단됐다. 처음의 sandbox CLI 호출 형식 오류는
권한 증거에 포함하지 않았고, 설치된 도움말의 `--permission-profile` 형식으로
수정하여 위 결과를 얻었다.

실제 모델의 제한된 shell 요청 검사는 `var/telegram-compat-j3sbargn`의 독립 홈에서
실행했다. 이는 시나리오 검증이며 모든 도구·공격·호스트에 대한 격리 증명은 아니다.
CLI 사용 방식은 설치된 `exec --help`, `features list`, `sandbox --help`와
공식 [Codex exec 문서](https://learn.chatgpt.com/docs/developer-commands#codex-exec)를
함께 확인했다.

## 빌드와 기존 검사

- 수정본 `go build -o bin/chunsu-telegram-fix ./cmd/chunsu` 성공.
- 최종 `go build -o bin/chunsu ./cmd/chunsu` 성공. 사용자가 실행하는 파일에 반영했고,
  최종 바이너리의 `doctor`에서도 reception prerequisites_match를 확인했다.
- `go vet ./internal/executor ./internal/cli ./internal/reception ./internal/telegram` 통과.
- 기존 `go test ./internal/runner` 통과.
- 변경 문서의 로컬 링크 31개 확인, 누락 없음. `git diff --check` 통과.
- 새 테스트 코드를 추가하지 않았다. 기존 Telegram 검사 도구를 재사용했다.

## 적용과 남은 범위

수정과 검증 문서는 IN-05 로컬 커밋의 범위다. 사용자가 실행한 기존
Telegram 프로세스는 실행 당시 바이너리를 계속 사용하므로 실행 터미널에서
Ctrl-C 후 `bin/chunsu telegram`을 다시 실행해야 한다. token/본인 페어링은
재사용하며 실패한 메시지는 다시 보내야 한다.

실제 사용자 봇에서 수정 후 답장, 장기 네트워크 장애, 새 버전의 Linux·다른 CPU,
보고서·review 새 실행기 재검증, 전체 프로젝트 테스트와 race detector는 이번
검사에 포함하지 않았다. 현재 버전에서 대화 외 경로는 계속 별도 재검증이 필요하다.
공개 배포·원격 push·백그라운드 등록은 수행하지 않았다.
