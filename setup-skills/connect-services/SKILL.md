---
name: connect-services
description: Prepare or guide Chun-su setup for Google Drive Docs and Sheets, Figma, GitHub, Slack, Telegram, and Gmail using the bundled service manuals and offline planner.
---

# 외부 서비스 설정 도우미

이 Skill은 Chun-su의 **설정 안내용**이다. 일반 메일/Jira 보고 실행기의 Skill과
다른 자산이다. 현재 이 파일을 보고 실행기에 넣거나 전역 실행 권한을 풀지 않는다.
현재 Gmail 읽기만 native 지원된다. 나머지는 공식 설치 경로를 조사한 매뉴얼이며
Chun-su의 공통 installer/adapter는 구현 전이다.

## 진행

1. 사용자가 원하는 서비스·기능과 이번 작업이 준비인지 실제 연결인지 확인한다.
   이미 답한 선택·기존 승인은 반복해서 묻지 않는다. 여러 서비스를 선택해도
   선택하지 않은 서비스는 설치하지 않는다.
2. 아래 해당 매뉴얼만 읽는다. 기존 연결을 재사용할 수 있으면 계정과 실제 권한을
   확인한다. 다른 AI 도구의 로그인 상태가 Chun-su의 권한이라는 가정은 하지 않는다.
3. 해당 서비스의 질문을 작은 묶음으로 제시한다. 계정·파일·채널 같은 범위는
   사용자가 선택하고, 발견 가능한 ID는 승인된 조회로 확인한다.
4. 설치 경로, 필요한 의존성/권한, 사용자가 처리할 로그인 단계를 구체적으로 제시한다.
   새 권한 계약이 필요한 경로는 구현안으로 표시한다. 존재하지 않는 Chun-su 명령을
   만들어 실행하지 않는다.
5. 실제 설치가 요청·승인됐고 해당 경로가 지원되면 검토한 공식 설치법 또는
   호스트 helper를 사용한다. 이번 팩의 `prepare.py`는 **준비표 생성만** 한다.
   최신 upstream 코드를 내려받아 바로 실행하지 말고 선택 버전과 변경분을 확인한다.
6. 비밀값은 provider 로그인 또는 사용자의 로컬 비밀 입력으로 인계한다.
   토큰·OAuth code/callback URL·client secret을 대화/준비표에 붙여넣게 하지 않는다.
   공식 매뉴얼이 평문 export를 예시로 제시해도 그대로 적용하지 않는다.
7. 인증 identity와 허용 리소스의 실제 호출을 확인한 뒤 상태를 보고한다.
   도구 목록 발견, 로그인 성공, 기능 사용 가능, 룰셋 검증 완료를 각각 구별한다.
   전송/쓰기 검증은 해당 목적지와 내용에 대한 권한이 있을 때만 실행한다.

## 서비스 선택

- Drive·Docs·Sheets: [drive.md](references/drive.md). 파일 범위와 읽기/작성을 먼저 선택.
- Figma: [figma.md](references/figma.md). supported MCP client와 REST 경로를 구별.
- GitHub: [github.md](references/github.md). 저장소 범위와 개인용/배포용 인증을 구별.
- Slack: [slack.md](references/slack.md). 봇 대화 채널과 검색/자료 읽기를 구별.
- Telegram: [telegram.md](references/telegram.md). 봇 토큰 소유·기존 listener·사용자 페어링 확인.
- Gmail: [gmail.md](references/gmail.md). 기존 native 읽기 경로 우선; 전송은 별도 기능.

## 오프라인 준비 helper

Skill 디렉터리 기준으로 `python3 scripts/prepare.py --interactive`를 실행한다.
또는 `--services drive,gmail --mode assisted --intent read`처럼 선택한다.
helper는 카탈로그와 PATH상의 실행 파일 존재만 확인한다. 계정·인증파일을 읽거나
프로그램을 실행하지 않는다. `--output` 지정 시 새 파일만 생성한다.
질문에 취소하거나 EOF가 오면 답을 추정하지 않는다.

## 완료·복구 보고

각 서비스에 선택 경로, 설치/인증/범위 확인 결과, 실제 검증한 기능, 미지원 기능,
사용자에게 남은 한 단계와 해제 방법을 남긴다. 계정 연결은 룰셋 채택을 뜻하지 않는다.
외부 실행기 직접 연동은 호스트에서 동작을 관찰·제한한 증거가 없으면 편의 경로로
표시하고 검증된 Chun-su 실행과 혼동하지 않는다.

같은 원인의 동일 접근이 두 번 실패하면 중단하고 마지막 진단과 필요한 결정을
남긴다. 기존 연결을 삭제하거나 권한을 확대하는 재설치는 복구 기본값이 아니다.
연동 버전·권한·도구 표면 변경은 이전 품질/격리 증거의 적용 범위를 다시 확인한다.

새 서비스를 추가할 때 매뉴얼과 catalog 인덱스부터 준비한다. 실제 adapter와
호스트 권한 검증 없이 이름만 추가해서 지원 완료로 표시하지 않는다.
