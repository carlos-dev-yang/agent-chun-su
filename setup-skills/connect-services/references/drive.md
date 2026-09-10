# Google Drive — Docs·Sheets 설치 매뉴얼

확인일: 2026-09-10. 상태: 외부 경로 조사 완료, Chun-su adapter 미구현.
목표 기능: 파일 검색·지정 문서 읽기, Docs 생성/편집, Sheets 범위 읽기/쓰기,
선택 파일 내보내기. 공유/삭제는 별도 관리 기능으로 취급한다.

## 사용자에게 물을 내용

- Docs, Sheets 중 무엇이 필요한가? 읽기만 할지, 생성/수정도 할지?
- 개인 Google 계정인가, 회사 Workspace인가? 기존 승인된 OAuth client가 있는가?
- 먼저 사용할 문서/시트 링크 또는 새 결과를 둘 위치는 어디인가?
- Gmail도 연결한다면 같은 계정인가? 같은 계정이어도 Gmail의 권한을 자동 추가하지 않는다.

처음에는 파일 링크를 받아 account/파일/탭을 확인한다. 전체 Drive 탐색은 별도
선택이다. 로그인만 마친 상태에서 파일 내용 전체를 자동 수집하지 않는다.
새 OAuth client가 필요하면 [Google 공통 로그인 준비](google-auth.md)를 따라간다.

## 경로 A — AI 안내 + gws 호스트 wrapper 후보

`gws`는 Drive/Docs/Sheets를 제공하는 공개 CLI다. prebuilt binary 또는 npm 등으로
설치할 수 있고, `gws auth setup`은 gcloud를 필요로 한다. 이미 있는 Cloud project와
Desktop OAuth client를 사용하는 수동 경로도 있다. 공식 지원 Google 제품은 아니다.
[프로젝트 설치·인증](https://github.com/googleworkspace/cli).

설정 도우미가 할 일:

1. 설치된 gws와 지원 환경을 확인하고 설치할 release·해시·변경 위치를 정한다.
2. Google Cloud에서 선택 API(Drive/Docs/Sheets)를 활성화하고 OAuth audience와
   client를 준비한다. client JSON은 로컬 보호 파일로 받는다.
3. 로컬 OAuth에서 선택 기능에 맞는 권한을 확인한다. `gws auth login -s drive,docs,sheets`
   의 서비스 선택은 정확한 읽기 전용 scope 검증을 대신하지 않는다.
4. keyring 사용 여부와 실제 scope를 확인한다. credential export나 평문 fallback을
   기본 설정으로 쓰지 않는다. 기존 Chun-su Gmail token을 gws 파일로 복사하지 않는다.
5. host adapter가 gws를 고정된 인자·메서드로 호출하고 응답을 보존하도록 구현한다.
   현재 일반 보고 실행기에게 shell/gws 명령을 직접 주는 방식은 지원하지 않는다.

## 경로 B — 고정 helper/API 또는 공식 MCP

장기 기본 UX는 배포자가 준비한 OAuth 앱에 사용자가 로그인하고 파일을 선택하는
방식이다. 개인용 준비에서는 사용자의 Desktop client를 사용할 수 있다.
PKCE와 임시 loopback callback을 쓰는 native 흐름은 기존 Gmail 구조를 참고한다.
클라이언트 유형과 redirect는 실제 사용하는 흐름에 맞춰야 한다.
[Google 자격증명](https://developers.google.com/workspace/guides/create-credentials),
[native OAuth](https://developers.google.com/identity/protocols/oauth2/native-app).

Google 공식 원격 MCP는 별도의 Developer Preview 경로다. 참여 조건, Cloud API/MCP
활성화, OAuth client와 지원 도구 목록을 먼저 확인한다. 개발 문서를 검색하는
MCP와 실제 사용자 Docs/Sheets를 읽는 MCP는 다르다.
[Workspace MCP](https://developers.google.com/workspace/guides/configure-mcp-servers).

| 기능 | 검토할 OAuth scope | 중요한 범위 차이 |
|---|---|---|
| 선택 파일 사용·생성 | `drive.file` | 앱으로 열거나 Picker 등으로 선택된 파일 범위. 쓰기도 포함하며 URL 입력만으로 권한이 새로 생기지 않음 |
| 기존 Docs 읽기 | `documents.readonly` | 계정이 접근 가능한 문서 범위; 호스트에서 지정 document 제한 필요 |
| 기존 Sheets 읽기 | `spreadsheets.readonly` | 개별 탭 단위 OAuth 제한이 아님; 호스트에서 spreadsheet/tab/range 제한 |
| 문서·시트 작성 | `documents`, `spreadsheets` 또는 해당 메서드의 `drive.file` | 메서드별 허용 scope 확인, 선택 목적지 외 쓰기 차단 |
| Drive 전체 검색/내용 | `drive.metadata.readonly` / `drive.readonly` | 메타데이터와 본문 권한 구별, 넓은 권한은 필요한 경우만 |

[Drive scope](https://developers.google.com/workspace/drive/api/guides/api-specific-auth),
[Docs scope](https://developers.google.com/workspace/docs/api/auth),
[Sheets scope](https://developers.google.com/workspace/sheets/api/scopes).

## 첫 검증과 룰셋 사례

아래는 앞으로 실행할 검증이지 완료 기록이 아니다.

- 지정 Doc을 읽고 문서 ID·탭·수정 버전과 원문을 보존한다. `includeTabsContent`와
  하위 탭 처리를 확인해 첫 탭만 읽고 전체 문서라 하지 않는다.
  [Docs tabs](https://developers.google.com/workspace/docs/api/how-tos/tabs).
- 지정 Sheet의 작은 A1 범위를 읽고 요청/반환 범위·값·수식 취급을 기록한다.
  쓰기는 값 해석 모드(RAW/USER_ENTERED)를 선택하고, 변경 전후 범위를 비교한다.
  [Sheets values](https://developers.google.com/workspace/sheets/api/guides/values).
- 허용 밖 파일/탭 요청은 host에서 막고, credential·다른 계정 자료는 실행기에
  보이지 않아야 한다. 확인용 읽기가 성공해도 전체 Drive 지원 완료로 표시하지 않는다.
- 골든 사례: 누락 탭, 빈 셀과 0, 수식과 표시값, 문서 내 지시 주입, 변경된 문서의
  오래된 결과를 구별한다. 쓰기 재시도가 중복 문서/행을 만들지 확인한다.

## 문제 해결과 해제

접근 거부는 API 활성화, test user, 조직 허용 정책, 실제 파일 접근권, scope를
순서대로 확인한다. `drive.file` 실패를 전체 Drive 권한으로 자동 바꾸지 않는다.
External/Testing 앱의 refresh token 7일 만료 조건을 사전에 알려 재연결 안내로
처리한다. [OAuth 만료](https://developers.google.com/identity/protocols/oauth2#expiration).

해제는 먼저 host 연결을 비활성화한 뒤 해당 앱 grant/credential을 철회한다.
다른 Google 연결과 grant를 공유한다면 영향을 설명한다. 기존 문서·로컬 증거를
자동 삭제하지 않는다. 이번 팩은 API 활성화·앱 등록·OAuth를 실행하지 않았다.
