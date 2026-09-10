# Gmail 설치 매뉴얼

확인일: 2026-09-10. 상태: **Chun-su native 읽기 connector 구현됨**.
대화형 초기 설정 통합과 draft/send/modify는 미구현이다. 이전 사용자의 연결과
검증 결과를 새 사용자의 계정 연결 상태로 간주하지 않는다.

## 사용자에게 물을 내용

- 정확히 어느 계정을 연결할 것인가? 기존 Chun-su 연결 또는 Desktop OAuth client가 있는가?
- 읽기만 할지, 향후 초안·전송도 필요한지?
- 메일 조회 조건·기간·배치 크기·시간대는 무엇인가? 관련 thread 본문 확장이 필요한가?

이전에 선택한 값은 재사용한다. 신규 사용자의 조회 범위를 이전 pilot의 계정·7일·20건으로
자동 확정하지 않는다. 대화에는 token/secret 대신 보호된 client 파일의 위치만 받는다.
client가 없으면 [Google 공통 로그인 준비](google-auth.md)부터 안내한다.

## 경로 A — AI 안내 + 기존 Chun-su 명령

현재 실행 가능한 native 경로다. 실행 전에 실제 binary와 `gmail --help`를 확인한다.
아래 대문자는 사용자 선택값으로 대체할 자리이며 준비 helper가 실행하지 않는다.
개발 checkout의 `bin/chunsu`는 빌드 산출물 예시다.

```sh
bin/chunsu gmail list
bin/chunsu gmail keychain-check
bin/chunsu gmail connect --client CLIENT_JSON --account ACCOUNT --query QUERY --batch COUNT --timezone IANA_ZONE
bin/chunsu gmail check CONNECTION_ID
```

`keychain-check`는 임시 비밀이 아닌 marker의 저장·읽기·삭제를 실제 실행한다.
`check`는 계정/갱신 확인이고 본문 수집은 아니다. 기존 연결이 맞으면 connect 대신
check로 시작한다. 승인된 범위에서 `gmail collect CONNECTION_ID`로 원문을 보존한
뒤 queue/review를 선택한다. read-only 보고 실행기에는 이 설치 명령을 주지 않는다.
[현재 구현과 상세 복구](../../../docs/setup/GMAIL_PILOT.md).

native OAuth는 Desktop client, PKCE/loopback callback과 macOS Keychain을 쓴다.
일반 설정에 refresh token을 저장하지 않으며 현재 scope는 `gmail.readonly` 하나다.
지원되지 않는 비밀 저장 환경에서 평문으로 우회하지 않는다.

## 경로 B — 고정 helper 후보 / 다른 Google 도구

공통 설치 helper는 기존 `gmail list/check/connect/reauth`를 감싸서 같은 질문·결과
표시를 제공하는 방향이다. 신규 OAuth 구현을 중복해서 만들 필요는 없다.

gws와 공식 Gmail MCP도 대안으로 조사했지만 기존 native 계정/기록/갱신 경로를
대체하려면 별도 adapter 검증이 필요하다. gws 설치나 다른 AI의 Gmail plugin 설치가
기존 Chun-su의 live 승인과 동일한 것은 아니다.
[gws](https://github.com/googleworkspace/cli),
[공식 MCP 설정](https://developers.google.com/workspace/guides/configure-mcp-servers).

## 필요한 기능과 권한

| 기능 | 제공자 scope 후보 | Chun-su 상태 |
|---|---|---|
| 본문 읽기·조회 | `gmail.readonly` | 구현됨, 계정별 query/배치 제한은 host에서 적용 |
| 메일 전송 | `gmail.send` | 미구현; 수신자·최종 내용·전송 결과 계약 필요 |
| 원격 draft 관리 | `gmail.compose` | 미구현; 이 scope는 전송도 포함하므로 draft-only는 host가 제한 |
| 읽음/라벨/보관 등 변경 | `gmail.modify` | 미구현; 별도 선택과 검증 |

[Gmail 권한](https://developers.google.com/workspace/gmail/api/auth/scopes).
초안은 우선 로컬에서 작성할 수 있다. Gmail 서버에 draft를 저장하는 것은 실제
외부 쓰기다. Drive와 같은 Google 계정을 선택해도 기존 read-only token에 권한을
자동 추가하지 않는다. 필요한 기능에 맞춘 새 consent와 credential 분리를 검토한다.

## 첫 검증과 룰셋 사례

- 계정 identity·refresh·실제 query와 배치 경계를 확인한다. 빈 결과와 접근 실패를
  구별하고 수집 누락·첨부 미지원은 partial/unknown으로 남긴다.
- 원문을 AI에 전달하기 전에 실제 사용 실행기 조합의 synthetic 경계 증거와
  해당 계정 전달 권한을 확인한다. `live_mail_approved`를 설치 편의를 위해 자동 켜지 않는다.
- 기존 mail 골든 기준을 재사용하되 account 연결 smoke check를 의미 평가로 세지 않는다.
  읽음 변경/전송 금지, 주입 지시 무시, 일정·중요도·완료 여부의 근거를 확인한다.
- 전송을 추가할 때는 지정 검증용 수신자와 내용, 회신 thread/중복 전송을 검증한다.
  이 매뉴얼 작성 중 실제 Gmail 조회나 전송을 수행하지 않았다.

## 문제 해결과 해제

OAuth 실패는 client 유형·API 활성화·test user·조직 정책을 확인한다.
Keychain 실패는 기존 가이드의 진단을 따르고 비밀 저장을 우회하지 않는다.
External/Testing 앱의 refresh token 만료 조건 때문에 재인증 안내가 필요할 수 있다.
[Google OAuth](https://developers.google.com/identity/protocols/oauth2#expiration).

`gmail reauth CONNECTION_ID`는 같은 연결 복구에 사용한다. `gmail disconnect CONNECTION_ID`
는 로컬 연결 비활성화/credential 제거, `--revoke`는 Google grant 철회까지 수행한다.
기존 원문·보고서는 남는다. 다른 기능과 grant를 공유하는 설계로 바뀌면 철회 영향을
먼저 보여줘야 한다.
