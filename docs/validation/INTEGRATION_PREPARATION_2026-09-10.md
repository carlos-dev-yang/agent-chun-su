# IN-00 — 6개 서비스 연결 준비 검증

날짜: 2026-09-10. 기준 코드: `2db73c4` 이후 현재 체크아웃.
범위: 온라인 조사, 내부 설정 Skill·6개 매뉴얼·Google 공통 인증 안내,
Slack manifest 예시, 오프라인 준비표 helper와 구현 작업 단위.

## 실제 수행

- Hermes/OpenClaw와 Google/Figma/GitHub/Slack/Telegram 공식 문서·공개 소스를
  조회했다. 앱 설치나 실제 계정 접근 없이 인증/도구/배포 조건을 확인했다.
- `prepare.py --list`: 6개 서비스와 현재 지원 상태 표시.
- Drive+Gmail, AI 안내/읽기 모드: 공통 Google 계정 질문과 해당 매뉴얼만 포함.
- 전체 서비스, scripted/write 모드: 6개 서비스가 포함된 준비표 생성. write는
  실행 승인이 아니라 희망 기능으로 표시한다.
- Telegram+Slack 대화형 선택: 두 서비스만 포함되고 Gmail은 제외됨.
- 건너뛰기, 취소, EOF: 파일·설정 변경 없이 종료. 취소/EOF는 130 반환.
- 잘못된 서비스 ID: 오류로 종료. 기존 출력 파일 재사용: 거부하고 원래 바이트 유지.
- 새 출력 파일은 검증 host(macOS)에서 0600으로 생성. 임시 검증 산출물은 정리했다.
- Slack manifest는 JSON 파싱과 공식 필드 대조를 수행했다. 실제 Slack manifest
  validation API 또는 앱 설치를 통한 검증은 수행하지 않았다.

helper는 Python 표준 라이브러리만 사용한다. 코드 확인상 네트워크·subprocess·
credential 조회가 없고, 선택 도구의 PATH 존재를 검사할 뿐 실행하지 않는다.
PATH에서 미발견이라고 프로그램이 시스템 어디에도 없다는 뜻은 아니다.

## Skill 형식 검사 한계

skill-creator의 `quick_validate.py`를 시스템 Python과 번들 Python에서 각각
시도했으나 둘 다 `ModuleNotFoundError: No module named 'yaml'`로 실행되지 않았다.
같은 원인의 추가 재시도나 패키지 설치는 하지 않았다. 이 자동 검사를 통과했다고
기록하지 않는다. 정식 validator 재실행에는 PyYAML이 있는 검증 환경이 필요하다.

대신 frontmatter의 name/description, 폴더명, UI metadata 필드, 매뉴얼 연결을
직접 확인했다. 독립 AI가 실제 계정을 연결하는 행동 검증은 하지 않았다.

## 하지 않은 검증과 남은 작업

- 새 Go/테스트 파일은 만들지 않았다. helper의 직접 실행 시나리오만 확인했으며
  전체 Go 테스트는 영향 범위가 아니어서 실행하지 않았다.
- 실제 OAuth·PAT·bot token 입력, 외부 package 설치, provider resource 조회·쓰기,
  Slack/Telegram 메시지 전송, background service, executor 권한 변경은 하지 않았다.
- 6개 서비스의 설치 안내 자산이 준비된 것이며 6개 native connector나 자동 installer가
  동작하는 상태는 아니다. 현재 native 지원은 이 목록 중 Gmail 읽기다.
- 룰셋/도구 버전과 연결되는 독립 평가 실행, 설치용 host helper와 agent 권한 경계,
  배포용 OAuth 앱과 초기 쓰기 범위는 [구현 준비안](../implementation/INTEGRATION_ONBOARDING.md)의
  IN-01 이후 결정/구현 사항이다. Phase 7·MS5 완료를 주장하지 않는다.

변경 범위 Markdown 15개에서 로컬 링크 75개와 코드 블록의 닫힘을 확인했고
누락이 없었다. `git diff --check`가 통과했다. 실제 외부 링크의 내용 확인은
온라인 조사로 수행했으며 모든 제공자의 인증 기능이 동작한다는 검증은 아니다.
