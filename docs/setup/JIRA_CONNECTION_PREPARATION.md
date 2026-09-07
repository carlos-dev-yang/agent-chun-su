# Jira 실제 연결 준비

확인일: 2026-09-07.

이 문서는 Jira 실제 연결을 시작하기 전에 필요한 선택과 입력, 이후 구현 및
검증 순서를 정리한다. 토큰 발급, 계정 연결, 권한 선택 또는 네트워크 요청을
수행하는 절차가 아니다. 이번에 실행한 확인과 결과는
[2026-09-07 상태 기록](../implementation/STATUS_2026-09-07.md)에 별도로 남겼다.

## 현재 위치

Phase 6의 저장 응답 기반 수집 하위 범위는 구현되어 있다. `Reader` 경계,
Cloud-v3/REST-v2 형태의 저장 응답 읽기, 무연결 상태, Jira 전용 정규화,
원문과 스냅샷 보존, 재개와 재정규화, 조회 명령이 확인되었다. 이 범위는
P6-01a, P6-02a, P6-03a, P6-03b, P6-03c에 해당한다.

실제 HTTP reader와 인증 수명주기는 구현되지 않았다. `jira status`는 live
reader를 `not_configured`, 공급자 선택을 `pending`으로 표시한다. 따라서
P6-02b와 실제 tenant 검증은 미완료이며, Jira 보고서 생성과 AI 실행 경계인
P6-04 이후 작업도 시작되지 않았다.

## 비밀값 없이 준비할 입력

### 이번 대화에서 확인된 범위

- 배포: Jira Cloud, API 직접 연동.
- 사이트: `https://cjenm.atlassian.net/`.
- 프로젝트: `SWMPFE`, 검토 보드: `1704`.
- 보드 주소: <https://cjenm.atlassian.net/jira/software/c/projects/SWMPFE/boards/1704>.
- 목적: 본인 할당 업무와 To Do·2주 내 처리 조건에 맞는 보고서 준비.
- 이후 방향: 보고서에서 다음 수행 업무를 정하고 나중에 코드 작업으로 연결.
  현재 요청은 개발 executor 실행이나 이슈 변경까지 승인하지 않는다.

대상 조건은 사용자에게 확인 중이다. **본인 할당 AND To Do AND 14일 내 마감**인지,
**본인 할당 전체 OR (To Do AND 14일 내 마감)**인지 확정 전에는 실행 JQL을
만들거나 둘 중 하나로 수집하지 않는다. 두 번째 방식은 현재 저장 정책의
`assigned` / `assigned_or_sprint`만으로 정확히 표현되지 않으므로 정책 확장이
필요하다. 보드 선택을 현재 스프린트 선택으로 바꾸지 않는다.

‘2주 이내’가 Jira 기본 due date인지 별도 일정 필드인지 확인해야 한다.
시작·종료일 포함 여부, 이미 기한이 지난 이슈와 날짜 없는 이슈 처리,
보고 기준 시간대도 실제 수집 전에 명시한다. `To Do`는 해당 보드 열의 이름,
개별 status, status category 중 무엇인지 보드 설정으로 확인한다. 보드의
필터와 상태 매핑을 확인해야 하며, project 조건만으로 보드와 같다고 가정하지 않는다.

인증은 아직 설정되지 않았다. 개인 수동 API 연동의 email/API token 방식을
우선 검토하되 실제 token 유형과 읽기 scope는 필요한 endpoint에 맞춰 결정한다.
scoped token이면 Cloud ID와 API gateway를 사용한다. Jira 비밀정보 등록 기능을
먼저 구현하고 토큰은 로컬 비밀 입력 → Keychain에 저장한다. 채팅, 명령행 인자,
Git 파일에 토큰을 넣지 않는다. 기존 Gmail 자격증명으로 Jira를 인증할 수 없다.
본인 account ID, Cloud ID, 보드 필터·상태·날짜 필드 및 실제 접근 권한은 아직
조회하지 않았다.

아래 표에는 토큰, 암호, client secret 같은 비밀값을 적지 않는다. 인증 정보는
선택 이후 호스트 Keychain의 불투명한 참조로만 연결한다.

| 구분 | 준비할 입력 | 시점 |
|---|---|---|
| 배포 형태 | Jira Cloud로 확인됨 | 결정 완료 |
| 서비스 주소 | 위 사이트로 확인됨. scoped token이면 `cloudId` 및 API gateway 설정 필요 | 사이트 확인 완료, API binding 미설정 |
| 인증 방식 | 개인 수동 pilot용 email/API token 검토, 토큰 유형·읽기 scope 또는 필요한 경우 3LO | 세부 방식 미확정 |
| 적용할 사용 조건 | 회사 계정이면 이미 정해진 도구·인증 제한, 수집 자료 보존 및 AI 전달 범위 | 해당 조건과 미결 데이터 정책 확인 |
| 프로젝트 범위 | `SWMPFE` / 보드 `1704` | 사용자 지정 완료, 실제 보드 설정 미조회 |
| 선택 범위 | `assigned` 또는 `assigned_or_sprint`, 관련 이슈 문맥을 읽을 수 있는 범위 | 필수 입력 |
| 보드·스프린트 | 포함할 board/sprint 식별자. `assigned_or_sprint`에는 명시적 sprint ID가 필요 | 범위에 따라 필수 |
| 사용자 식별 | 대상 사용자의 provider-native stable ID와 identity field (`accountId`, `key`, `name`) | 연결 후 발견 가능, 사용 전 확정 |
| 필드 매핑 | sprint와 story points의 실제 `customfield_N` ID, 필요한 시간 추정 필드 | 연결 후 발견 가능, 수집 전 확정 |
| 수집 제한 | 페이지·이슈·history 항목 수, 응답 및 전체 증거 byte 한도, timeout/retry 한도, IANA timezone | pilot 전 확정 |

주소, 인증 방식과 조직 정책은 서로 독립된 입력이 아니다. 예를 들어 Cloud의
scoped API token은 만료일을 확인해야 하며, Atlassian 문서가 지정한 API
gateway 경로를 사용한다. Data Center PAT는 제품 버전과 조직 설정에 따라
사용 가능 여부가 달라지므로 실제 환경에서 확인해야 한다.

## 확정 전 제안

첫 실제 연결은 한 사용자와 소수 프로젝트를 대상으로 한 수동 read-only
pilot으로 제한하는 방안을 제안한다. 자동 일정이나 상시 동작은 포함하지
않고, 요청 route와 반환 필드를 검토할 수 있는 크기로 페이지와 이슈 수를
고정한다. 이 제안은 공급자, 인증 방식 또는 권한을 대신 결정하지 않는다.

pilot의 첫 산출물은 기존 수집 경계에 맞춘 원문 응답과 정규화 스냅샷이다.
원문을 먼저 보존하고 provenance, 누락 필드, 부분 pagination을 확인한다.
이 데이터를 AI Jira 보고에 전달하는 일은 별도 단계다. 수집 성공을 보고서
완성이나 업무량 판단으로 기록하지 않는다.

## 코드 연결 지점과 필요한 확장

`internal/jira/reader.go`의 `Reader.ReadPage`와 `Page`는 provider HTTP reader가
연결될 자리다. `internal/jira/acquire.go`의 수집 루프는 reader가 반환한 원문을
먼저 저장한 뒤 기존 normalizer에 전달하므로 재사용할 수 있다. 정책, 매핑,
SQLite acquisition index, private evidence files와 provider별 조회도 유지한다.
`internal/secrets/keychain.go`의 불투명한 credential reference 저장 방식과
plaintext fallback 금지 원칙도 재사용할 수 있다.

다만 현재 `CollectSaved`, `Load`, `Resume`는 `SavedReaderKind`와 보존된 전체
`SavedInput` 원본을 전제로 source/policy, cursor 진행과 page metadata를 다시
검증한다. HTTP reader만 추가하면 live acquisition의 load/resume은 성립하지
않는다. 선택된 provider profile의 digest, reader kind, 고정된 scope와 mapping,
continuation을 checkpoint에 결합하고, 보존된 각 HTTP 응답으로 무결성과 진행을
검증하는 live 경로가 필요하다. 기존 saved acquisition의 검증 의미와 불변성은
그 과정에서도 유지해야 한다.

또한 Jira connection/profile 저장 형식과 inactive 기본 상태, 승인 origin과
read route allowlist, 인증 부착 위치, 명시적 오류 분류가 아직 없다. 공용
Keychain의 현재 오류 문구 중 Gmail에 한정된 표현은 Jira가 사용할 때 공용
의미로 정리할 필요가 있다. Gmail의 endpoint와 client 구현은 provider별 계약이
다르므로 Jira 구현에 그대로 사용하지 않는다.

## 선택 이후 작업 순서

현재는 Cloud 선택까지 완료됐다. 아래 연결 profile과 외부 인터페이스의 변경안은
읽기 조건을 확정한 후 검토할 제안이며, 이미 구현된 기능 목록이 아니다.

1. 배포 형태, 실제 주소, 인증 방식과 조직 정책을 기록하고 연결 profile을
   비활성 상태로 만든다. profile 저장만으로 요청이나 Keychain 읽기를 하지
   않는다.
2. 기대 사용자, project/board/sprint 범위와 최대 수집량을 고정한다. 발견이
   필요한 identity와 field ID를 얻는 read route도 같은 allowlist에 포함한다.
3. 선택된 배포 한 종류의 read-only HTTP adapter를 구현한다. 승인된 HTTPS
   origin과 route만 허용하고 redirect, 임의 URL과 모든 write endpoint를
   거부한다.
4. 인증·권한·rate limit·일시 오류·잘못된 응답을 구분하고, pagination과 retry를
   고정된 한도 안에서 처리한다. 지정 보드 수집은 enhanced
   `GET /rest/software/1.0/board/{boardId}/issue`와 추가 JQL 조건을 우선 검토한다.
   보드 설정은 `GET /rest/agile/1.0/board/{boardId}/configuration`으로 확인한다.
   일반 검색이 필요하면 `/rest/api/3/search/jql`을 검토하되 보드 범위를 유지한다.
   두 검색 응답의 실제 필드·페이지 형태가 현재 normalizer와 맞는지 따로 확인한다.
5. live acquisition의 생성, checkpoint, load와 resume 검증을 saved 방식과
   구분해 연결한다. 원문 우선 보존과 기존 정규화 계약은 바꾸지 않는다.
6. controlled fake transport로 아래 증거를 확보한 뒤, 승인된 경우에만 최소
   수동 pilot을 수행한다. AI 전달과 보고 평가는 별도 작업으로 검토한다.

## P6-02b 완료 증거

- 요청 method, origin, path, JQL, fields가 승인된 값과 일치하고 credential이
  해당 route에만 붙는다는 검토 기록
- 다른 host/path, redirect, caller가 제공한 URL과 Jira를 변경하는 endpoint가
  거부된 결과
- 정상, 빈 결과, 마지막 page, 반복·누락 continuation, page/issue/byte 한도의
  controlled transport 결과
- 401, 403, 404, 429와 `Retry-After`, 5xx, timeout, 잘못된 JSON, 큰 응답이
  구분되고 retry가 한도를 넘지 않는 결과
- 로그인 identity 불일치와 scope 밖 project/related context가 차단되고,
  발견하지 못한 field mapping이 0으로 추정되지 않고 unknown/partial로 남는 결과
- live 형태 응답이 원문 우선 보존, 기존 normalizer와 provenance를 통과하며,
  resume에서 고정된 profile/policy/mapping 변경을 거부하는 결과
- 기존 synthetic Cloud-v3/REST-v2 collect/resume/show와 직접 영향받은 Gmail
  acquisition listing 및 backup 동작의 회귀 확인
- 실제 tenant 로그인이 수행되지 않았다면 tenant metadata, provider 동작과
  실제 권한은 미검증이라고 명시한 기록

읽기 전용 여부는 HTTP method만으로 판단하지 않는다. Cloud 검색의 POST처럼
데이터를 변경하지 않는 요청도 있으므로 method와 route 조합을 검토한다.
자동 테스트 코드는 사용자 요청이 있을 때만 추가하고, 그 전에는 기존 검사와
수동 검증으로 증거를 남긴다.

## 2026-09-07 공식 확인 자료

- [Jira Cloud board API](https://developer.atlassian.com/cloud/jira/software/rest/api-group-board/): enhanced 보드 이슈 조회는 token pagination을 사용한다. 보드 설정에 filter와 열별 status 매핑이 포함된다. 권한이 없는 보드 조회의 빈 결과를 본인 업무가 없는 것으로 오해하지 않도록 계정·보드 접근 확인을 먼저 한다.
- [Jira Cloud issue search](https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/): 기존 search endpoint가 제거 중이며 enhanced `GET/POST /rest/api/3/search/jql`과 `nextPageToken` pagination을 제공한다.
- [Atlassian API token 관리](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account/): scoped API token의 API gateway 경로와 token expiry를 연결 전에 확인한다.
- [Jira Cloud REST basic auth](https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/): 개인 수동 script의 email/API token 방식과 앱 통합의 OAuth 2.0(3LO)을 구분해 선택한다.
- [Data Center Personal Access Token](https://confluence.atlassian.com/enterprise/using-personal-access-tokens-1026032365.html): PAT 지원 여부와 사용 조건을 실제 Data Center 버전 및 조직 정책에서 확인한다.

이 자료 확인만으로 구현 계약이 확정되지는 않는다. Cloud 선택은 확인됐고,
인증 방식의 세부사항·대상 조건과 pilot 범위를 확정한 뒤 상태 기록에 남긴다.
