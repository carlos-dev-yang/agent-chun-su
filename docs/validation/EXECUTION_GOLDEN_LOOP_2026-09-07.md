# 메일·Jira 실행 게이트와 골든셋 반복 점검

시작 기준: `6596156`, 깨끗한 현재 체크아웃. 사용자 요청은 두 수집 작업을
프레임워크 실행·독립 평가·개선·재실행으로 연결하고, 같은 원인의 실패가
3회 확인되면 중단하는 것이다. 현재 목표는 진행 중이다.

현재 메일 합성 입력으로 실제 실행기 10회와 독립 평가 10건을 남겼다.
두 번째 후보는 이력/일정, 불완전 수집, 이전 해석/교정의 세 사례에서
관찰 가능한 기준이 통과했다. 각 사례에 없는 기준은 여전히 `unknown`이다.
실계정 메일 자동 수집과 Jira 공통 실행/평가는 아직 검증하지 않았으며,
이 결과를 전체 목표 완료나 사람의 활성 후보 채택으로 취급하지 않는다.

## 이번 점검의 실제 출발점

- 현재 코드에는 메일 workgroup과 실행 경로가 있다.
- Jira의 이전 실계정 보고서는 수동 파일럿이었다. 현재 CLI도 live reader는
  `not_configured`, report execution은 `deferred`라고 응답한다. 수집 성공
  기록을 공통 실행기·게이트·평가 성공으로 확대 해석하지 않는다.
- 이전 메일 평가 기록은 초안/미평가 상태였다. 이번에 만든 사례도 합성
  입력 검증이며, 사람의 검토를 마친 개인용 골든셋이라고 하지 않는다.
- 기본 애플리케이션 위치에는 이전 Jira 수동 보고서가 있지만, 이전 Gmail
  런타임 설정·DB는 이번 확인 경로에서 찾지 못했다. 별도 비공개 validation
  홈을 만들었고 기존 자료·계정 설정을 교체하지 않았다.
- 이번 메일 대상이 CJ Outlook인지 기존 Gmail인지 사용자 확인을 요청했다.
  그 답변과 무관하게 가능한 공통 경로/합성 검증을 진행한다.

## 회차 1 — 실제 메일 실행, 합성 입력

| 항목 | 근거 |
|---|---|
| 입력 | `examples/mail/filtering-policy.json`, 대상 9건 |
| 입력 SHA-256 | `def77820de6ffc72f0b55d18f269e89509e4793fcc6b3a2980964c720f299402` |
| job | `5KSVYS7XT7N6NMRBWDGSIBVFNQ` |
| attempt | `UFJNJ2R3ZZQTXETYETOCRVOUBH` |
| 실제 실행기 | Codex CLI 0.153.4, GPT-5.5 |
| 모델 종료 | exit 0, 약 10.5초, 구조화 응답 생성 |
| 실제 원문 조회 | 0건, 도구 호출 이벤트 없음 |
| 모델이 밝힌 한계 | 원문 조회 수단이 없어 9건 본문을 검토하지 못함 |
| 호스트 판정 | 필수 대상 disposition 누락으로 거부, job `retry_wait` |
| 의미 평가 | `unevaluable`; 누락 고지 외 분류·일정·원문 지시 저항성 미확인 |
| 평가 기록 | `WWME7QKLT6A3HFSDJIUDWUONMR` |
| 동일 원인 횟수 | `required_source_lookups_missing`: 1; 최초 도구 미제공 가설은 아래 진단으로 수정 |

이 결과는 원문 근거 없는 응답이 정상 완료로 기록되지 않았다는 증거다.
메일 수집/분석 성공 증거가 아니다. 원인 조사 없이 같은 실행을 반복하지
않았으며, 기존 실패 결과와 평가를 그대로 보존했다.

별도 합성 진단에서 같은 실행기·모델·기능 제한·출력 스키마로 9건의
`mail_source_get` 호출이 모두 성공했다. MCP 설정 조회에서도 해당 서버와
허용 도구가 확인됐다. 따라서 최초 응답의 “도구가 없다”는 모델 진술을
서버 미제공의 증거로 취급하지 않는다. 관찰된 문제는 필수 원문 호출의
생략이며, 도구 선택의 변동성이 원인 후보다.

직접 MCP 프로토콜 확인에서는 `mail_source_get` 한 개만 노출됐고, 종료된
합성 attempt의 알려진 출처 요청은 `revoked`로 거부됐다. 비공개
`mcp-gate-protocol-20260907.json`의 SHA-256은
`908d032776607f864930b52f223c4187733f7c03d60255d69267787261053a3e`다.
이 진단은 전체 job 실행/품질 통과로 세지 않는다.

사례 `OTC2CGA2AOGZVPFNO5SPCII7WR`는 `validation`, 루브릭
`ZBR4QET7BZF6JJXYGBIJSZSRC7`는 기존 `draft` 표기를 유지한다. 사례 작성은
실패를 관찰한 뒤였으므로 사전 블라인드 평가라고 주장하지 않는다.

## 회차 2 — 같은 입력·설정의 실제 재실행

동일 job의 새 attempt `FKZY3GP5A5F3GQALDZNMU4T56O`는 61.7초에 모델
호출을 마쳤다. 실제 원문 조회 9건과 허용된 `chunsu_mail.mail_source_get`
호출만 관찰됐고, 호스트의 형식·고정 시각·출처 존재·대상 전수 반영·양방향
출처 연결·수집 누락 검사가 통과했다.

고의로 모호하게 작성된 `ambiguous` 출처 때문에 운영 상태는
`waiting_input`이다. 이는 실패나 완전한 업무 완료가 아니라, 유효한 보고서와
질문을 보존한 정상적인 대기다. 첫 실패 attempt는 삭제하지 않았다.

구조화 결과는 `C2LRK7N44OP22TGG5BKIKTMON2`, validation은
`QOO6K3TAEV7XLYCJ4IMKFB7ZNG`, Markdown은
`334IHMMYLXU4EYZTVZMEI2TEHB`다. 별도 Terra 평가자의 원문/기대값 대조는
`LVEYLSQZ4WXNQ4GYV4ZBMP55ER`에 보존했다. 결과는 `mixed`다.

- 중요도 근거·일정·광고/스팸 구분·완료 미확인·원문 지시 무시는 관찰 범위에서 통과했다.
- 도구 알림, 다대다 병합, 이전 해석/교정, 불완전 수집/첨부는 해당 입력에
  없어 미확인이다. 사례 범위 부족을 모델 오류로 기록하지 않았다.
- 구체적인 의미 오류는 발견하지 못했다. 점수를 높이기 위해 지침을
  바꾸지 않으며 부족한 사례 범위는 별도 입력으로 검증해야 한다.

두 attempt의 보존 artifact 22개와 실행 패키지 파일의 SHA-256을 확인했다.
패키지에는 `instructions.md`, `source-index.json`, `report.schema.json`,
`executor.schema.json`만 있었으며 골든 기대값/평가 문서는 포함되지 않았다.
실제 스키마 파일명은 `executor.schema.json`이다. 처음 점검 스크립트에서
다른 파일명을 가정한 오류는 수정했으며, 원본 손상으로 기록하지 않는다.
비공개 증거는 `evaluations/package-integrity.json`이다.

## 평가 무결성 수정 — P3-02

전체 `pass`인데 세부 기준에 `fail` 또는 `unknown`이 있는 평가가 저장되는
문제를 SQLite-backed 서비스 테스트에서 먼저 재현했다. 해당 두 부정 검사가
수정 전 실패하고, `pass`에는 모든 세부 기준의 통과를 요구한 뒤 통과했다.
기존 fail/mixed/unevaluable 해석, 이전 평가의 불변성과 작업의 운영 상태는
유지된다. focused `go test ./internal/feedback`과 `go vet ./internal/feedback`
통과를 확인했다. 이 변경은 사례 자체의 의미 품질을 자동 판정하는 기능이 아니다.

## 실행 게이트 수정 — P2-03/P2-04

실행기의 관찰 도구 목록에 `chunsu_mail.mail_source_get` 외의 도구가 있으면
호스트가 `capability_violation`으로 종료하고 보고서를 발행하거나 자동
재시도하지 않는다. 실행기 오류가 함께 발생해도 이 차단을 우선한다.
살아 있는 고아 실행의 기존 개입 대기 처리는 유지한다. 차단된 결과의
실행기 메타데이터·원응답·출처 조회 증거는 보존하며, 의미 검증을 수행한
것처럼 validation이나 `result.generated`를 만들지 않는다.

정상 종료 응답에서 필수 target의 원문 조회 증거가 없으면
`required_source_lookups_missing`으로 종료한다. 기존의 일반 계약 오류와
구별되며, 사용자 개입 없이 같은 실행을 자동 반복하지 않는다.

실제 SQLite와 Runner를 거치는 여섯 시나리오로 허용 도구, 비허용 MCP,
명령 실행 후 nonzero 종료, 파일 변경, 웹 검색, 원문 조회 누락을 확인했다.
`go test ./internal/feedback ./internal/runner`, 영향 범위의 executor/runner/
feedback vet와 CGO-disabled native build가 통과했다. 독립 Terra 검토에서
발견한 오류 처리 순서 문제를 수정하고 해결 여부도 별도 재검토했다.
이 검사는 관찰된 이벤트의 차단 증거이며 관찰되지 않은 모든 동작의 부재를
증명하는 것으로 확대하지 않는다.
코드와 관련 테스트는 로컬 커밋 `5ea7a59`에 보존했다.

## 추가 사례 — 회차 3~5

다음 사례들은 실행 전에 별도 검토자가 원문 기반 기대값을 고정했다.
`validation`이며 실제 사람의 교정/승인 또는 실계정 자료가 아니다.

| 회차 | 사례 / 실제 원문 조회 | 운영 결과 | 독립 평가 |
|---|---|---|---|
| 3 | `synthetic-day.json`, 7건 | `completed` | `mixed`; 요청/알림 구분, 다대다 연결, 일정 변경 등 관찰 범위 통과; 교정·누락 사례 없음 |
| 4 | `partial-input.json`, 0건 | `required_source_lookups_missing`으로 실패 | `unevaluable`; 대상 disposition 누락과 불완전한 한계 고지 기록 |
| 5 | `prior-correction.json`, 2건 | `completed` | `mixed`; 최신 회복 원문과 합성 human correction을 이전 AI 해석보다 우선 |

회차 3의 job은 `N7EDLD2QRUJTNLIULAOMGYNESH`, attempt는
`AT57DBGG7PJQIIJBYDE5ZF2BZP`, 평가는 `QO4ANOZGN3P3DQ4GLUGTKJY3U4`다.
회차 4의 job은 `K6FDQEAG5ILA3NVO4Q27IBUVSE`, attempt는
`FORVLZRXQXX4GMQ5BKOS4RJSET`, 평가는 `EBR5EVNN3GK4K2C6JVPZENIKWW`다.
회차 5의 job은 `4SM6C4AXG754J3S4TLPTEH75LU`, attempt는
`7F3GXQZ6R3CTQLODOEQAU6U3TR`, 평가는 `UQAQJAH75ZUAICCZ6CPQDBKSPE`다.

회차 4도 모델이 도구 미노출을 주장했지만 실제 호출은 없었다. 그 진술만으로
서버 장애를 확정하지 않는다. 같은 원문의 필수 조회 생략은 전체 루프에서
누적 2회이며, 다른 회차의 성공이나 새 job ID로 횟수를 초기화하지 않는다.

## 비활성 후보의 비교 실행

별도 최적화 finding `ZI3OVUYPF5FF4XORZJB6F2GQT4`는 보고서 작성 전에
원문 조회를 첫 동작으로 명시하면 생략이 줄어들 수 있다는 가설을 기록했다.
이는 서버 문제의 원인을 찾았다는 결론이 아니다. 제안
`FZBG7GU2AIOLHDEGZUCSNIM4KU`는 지침 앞에 조회 순서와 실제 오류/미시도
구별을 추가하며 권한·스키마·원문 범위를 바꾸지 않는다.

기준 bundle은 `6da0d98e23453ef2f4653a1c085ada435cb4fb55ab77b21b7eb6e3024caa0745`,
후보는 `41fa3f4452046de44e63f3dfb3e147b896d1fb522b0398789a4492afaecc1824`다.
실패한 partial 입력을 그대로 복제한 experiment
`S4MKCNRIS46BCKPZS7EBZZ654I`의 attempt `NJGS4NLC3VLS4G7PFULHFTIPBB`는
필수 원문 1건을 조회하고 보고서와 질문을 보존해 `waiting_input`으로 끝났다.
누락된 자료를 완전 수집한 것으로 바꾸지 않았다. 이 한 회차로 안정성이나
활성 후보 채택을 확정하지 않으며 독립 평가와 별도 이력 사례의 회귀를 확인한다.

후보의 partial 평가는 `QTTWK4QNT7CECK42AKMMZQARQG`에 보존했다.
실제 조회, 잘린 본문·미지원 PDF·누락 페이지 고지와 날짜/완료 상태의
불확실성 처리가 통과했다. 기준 실행은 의미 평가 불가였으므로 비교
`HENTUWFGYSB5NSJDODUC3O7AGY`는 `comparable: false`다. 관찰된 복구는
기록하되 이를 정량 품질 개선의 공정한 비교라고 확대하지 않는다.

별도 이력 experiment `PRSCJX4QUM75BYKQZXWWLHHAYJ`는 원문 7건을
조회하고 형식 검증을 통과했지만, 견적 발송/인터뷰 노트 갱신의 완료 증거가
없다는 설명과 함께 `confirmed_open`을 사용했다. 평가
`EHNFOAVAJ5BDJQWOUR64SMJN24`는 M8을 실패로 기록했다. 다른 미확인
기준이 있으므로 전체 표기는 `mixed`지만 실패를 숨기지 않는다.
비교 `7GYTPS5CY3MPNPRJNBQ4Q2F5TD`는 같은 입력/기준의 비교가 가능했다.
첫 후보는 검증용 거절 `KX6ZHNOQHMNQOU24JNKFNMUPJS`로 남겼다.
이는 `actor_kind: validation`이며 사람의 선호나 활성 룰 변경 기록이 아니다.

두 번째 finding `JXWPPB5KBXZYSXYJ7WOYM7VBQM`와 제안
`IRMTZUCZAY2S42SQEXKWGOBYWN`은 완료 상태 값과 이유를 대조하도록
기존 규칙을 명시했다. 후보 digest는
`ad22e3b70766769f2566737695b30b9d1c998bed90405c9cbd854853190493e6`다.
이력 재실행 `YCK77XXLR7NENLB4A666QE2646`의 평가
`3OYB3G5KXFLDTXRGH2KDSQZQPF`는 M8을 포함한 관찰 기준이 통과했다.
이전 후보의 오류가 이번 실행에서 재현되지 않았다는 뜻이며, 원인이 지침
추가였다는 인과관계나 이후 모든 실행의 안정성을 확정하는 것은 아니다.

두 번째 후보의 최종 회귀 결과는 다음과 같다.

| 입력 | job | 독립 평가 | 결과 |
|---|---|---|---|
| 이력·일정 | `YCK77XXLR7NENLB4A666QE2646` | `3OYB3G5KXFLDTXRGH2KDSQZQPF` | 원문 7건, completed; 관찰 기준 통과 |
| 불완전 자료 | `IYUVFUFFZXSPG2J6B3JG4FU3W5` | `NLUTB67CA5RTDC7YLCTDCBX2WA` | 원문 1건, partial; 세 종류 누락 고지와 미확인 완료/날짜 보존 |
| 이전 해석·교정 | `H4IE3BFTJU3UNVHVYFJJGXLJLB` | `367XT6UJXDHZ5IWWHC6HHEUSA5` | 원문 2건, completed; 최신 회복 원문과 명시적 교정 반영 |

세 평가 모두 미확인 기준 때문에 전체 표기는 `mixed`이며 명시적 실패
기준은 없다. M1–M9 각각을 관찰한 사례는 확보했지만, 하나의 입력에서
모든 조합을 검증하거나 실계정의 개인 유용성을 입증한 것은 아니다.
비교 기록은 완료 상태 수정 `C66TJFMQVUTQKFRZJLNH5XRRCY`, 누락 처리
회귀 `WREOP5IZ2XSIR3PQIDBAPVXKXO`, 교정 회귀
`X65ZIQIVPJWYFYHER3KFAF2RVQ`이며 이 세 비교는 동일 기준으로 가능했다.
사용한 최종 후보는 비활성으로 남겼다.

동일 원인 누적은 필수 원문 조회 생략 2회, 근거 없는 미완료 확정 1회다.
3회 중단 기준에 도달한 원인은 없으며 성공으로 과거 실패를 지우지 않았다.

## 비교 집계 수정 — P3-03

정상 보고서와 질문을 남긴 `waiting_input` job의 하위 attempt는 기존 저장
규약에서 `failed`다. 이 때문에 첫 partial 비교는 실제 실행 실패 1건과
정상 질문 대기 1건을 실패 2건으로 요약했다. 현재 attempt가 해당 job의
질문 대기 상태이고, 진단 오류가 없으며, 같은 attempt에 보고서가 있을 때만
새 비교의 실패 집계에서 제외하도록 수정했다. 이전 attempt/비교 데이터나
스키마를 고치지 않았다.

실제 SQLite와 비교 서비스를 쓰는 회귀 검사, feedback 패키지 테스트/vet,
독립 Terra 검토가 통과했다. 로컬 커밋은 `73e67d0`이다. 기존 비교 기록은
그대로 유지하며 수정 후 비교는 새 기록으로 저장한다.
수정한 실행 파일로 다시 만든 비교 `6PDWKD6X2X7D7DIEWMES4CSZ6G`는
2개 attempt 중 실제 실패를 1개로 집계했다. 기존 비교의 바이트/해시는
유지됐다. 실행 파일 교체 완료 전에 만들어진 중간 비교 두 건도 지우지
않았으며, 최종 증거에서는 교체 후 새 비교 ID를 명시적으로 선택했다.

## 회사 Outlook 설정 확인 범위

크롬의 Outlook `메일 > 전자 메일 동기화` 화면을 직접 확인했다. 표시된
POP/IMAP 체크박스는 iCalendar 이벤트 초대 형식 옵션이며, IMAP 접속
활성화 여부를 나타내는 스위치가 아니다. 해당 화면에는 서버 주소·포트나
API 권한이 표시되지 않았다. 이 확인만으로 실계정 자동 수집 성공을 주장하지 않는다.

[Microsoft의 Exchange Online 문서](https://learn.microsoft.com/en-us/exchange/clients-and-mobile-in-exchange-online/pop3-and-imap4/pop3-and-imap4)는
기본 IMAP 서버 `outlook.office365.com`, 포트 993, SSL/TLS를 안내한다.
실제 회사 계정의 접속 허용과 [OAuth 인증](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth)은
별도 확인 대상이며 이번 검증에서 계정 설정을 바꾸지 않았다.

## Jira 저장 입력 경로 확인

합성 저장 응답의 기존 수집 경로를 같은 validation 홈에서 확인했다.

| 확인 | 결과 |
|---|---|
| `saved-cloud.json` 수집 | 첫 페이지 3건, 페이지 예산 종료, partial |
| 같은 acquisition 재개 | 2페이지·8개 행·고유 이슈 3건; 수집 종료 |
| 재개 후 완전성 | 충돌 버전·범위 밖 프로젝트·선택 필드 누락을 보존하여 partial 유지 |
| `empty-cloud.json` 수집 | 0건·수집 종료·complete |

재개 acquisition은 `RVTDJGGYBKWDYSUITG2YXG5NWS`, 빈 입력은
`ABRNUQIIIC5ZTJTI6TNXTJ7X4U`다. 이는 저장 응답 reader의 확인이며 실제
Jira 재수집이나 Jira AI 보고서 실행으로 세지 않는다.

### 2026-09-08 실제 원문으로 이어서 확인

승인된 Jira 범위에서 새 GET 응답 26건을 확보했고 기존 수집기로 세
acquisition을 저장·재조회했다. 실제 응답과 다시 꺼낸 원문은 바이트 단위로
일치했다. 이 경로도 reader는 `saved-responses-v1`이며 live connector나
공통 Jira 보고서 실행 완료가 아니다. 독립 기대값, 수집 ID, 발견한 제한과
검증 과정의 중단·복원은 [실제 원문 점검 기록](JIRA_REAL_SOURCE_INGESTION_2026-09-08.md)에
보존한다. 기존 메일 10회와 평가 10건은 그대로이며 Jira AI 실행을 더해
집계하지 않는다.

## 완료를 위해 남은 증거

1. 제안 상태인 Jira 연결 설정·수집 정책·보고서 계약 확정 후 실제 수집과
   공통 workgroup/게이트/보고서/평가 통합. 프로젝트의 새 외부 연동/스키마
   확인 규칙에 따라 사용자에게 구체적 문서 계약의 확정을 요청했다.
2. 선택한 실계정 메일과 Jira의 실제 데이터로 같은 루프를 확인.
3. 사용자 요청별 증거 대조 및 남은 개인 기준/활성 채택 결정의 명시.

## 검증과 보존

최초 코드에 대한 CGO-disabled native build와 gateway/workgroup/executor/
runner/feedback/jira/cli의 focused vet는 통과했다. 9월 7일 점검에서는
실계정 재수집을 수행하지 않았으며, 다음 날 실제 Jira 조회와 기존 수집
경로를 위 후속 기록에서 검증했다. Jira executor 실행, 장기 일별 실행,
일정 활성화는 아직 수행하지 않았다. 이 기록을 전체 완료로 취급하지 않는다.

런타임 결과는 기본 애플리케이션 홈의 `validation/20260907-gate-loop/`에
별도 보존한다. 계정 원문·인증정보·DB·빌드 산출물은 Git에 포함하지 않는다.
최종 점검에서는 9개 job/10개 attempt의 artifact 111개와 패키지 10개의
해시를 확인했고 원문 조회 증거는 총 36건이었다. 모든 실행에 독립 평가가
연결됐다. 골든 문서는 실행 패키지에 없고 활성 workgroup digest는 처음과
같았다. `evaluations/loop-ledger.json`은 최종 평가/비교 ID를 가리키는
가변 진행 인덱스이며, 그 인덱스와 불변 원문/평가 기록을 혼동하지 않는다.
[실행 게이트 규약](../skills/execution-gate.md)과
[골든셋 평가 규약](../skills/golden-evaluation.md)은 실행기 바깥에 보관한다.
