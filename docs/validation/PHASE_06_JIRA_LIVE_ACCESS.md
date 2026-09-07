# Jira Cloud 수동 접근 확인 — 2026-09-07

사용자가 실제 보드 접근과 데이터 확인을 요청한 범위에서 실행했다.
이 기록은 일회성 API 조회 증거이며, 프로젝트의 live connector 구현 완료나
Jira report job 실행 증거가 아니다. 토큰·응답 원문·이슈 제목은 이 문서에 저장하지 않는다.

## 확인한 연결과 의미

- 사용자 지정 Keychain 항목 `chunsu-jira-cjenm`을 메모리로 읽어 API 인증에 사용.
  토큰을 출력하거나 파일·명령행 인자에 저장하지 않았다.
- 사용자 지정 사이트의 `/_edge/tenant_info`에서 Cloud ID를 확인하고
  `api.atlassian.com/ex/jira/{cloudId}`로 인증 요청을 보냈다.
- `/rest/api/3/myself`: HTTP 200. 응답 이메일이 사용자가 지정한 계정과 일치,
  active=true, 시간대 `Asia/Seoul`.
- `/rest/agile/1.0/board/1704`: HTTP 200. 프로젝트 `SWMPFE`, Kanban 보드 확인.
- 보드 configuration: HTTP 200. `TODO` 열은 status ID `10084`에 연결된다.
  실제 이슈 status 이름은 `Backlog`다. 문자열 `status = "To Do"`로 대체하지 않는다.
- `/rest/api/3/field`: HTTP 200. 기본 `duedate`는 `기한`,
  `customfield_10015`는 `시작 날짜`로 확인했다. 다른 날짜 필드의 존재만으로
  해당 보드의 업무 마감 기준이라고 판단하지 않는다.

## 읽기 범위와 관찰 결과

모든 이슈 조회는 enhanced board endpoint
`GET /rest/software/1.0/board/1704/issue`에
`project = SWMPFE AND assignee = currentUser()` 조건을 적용했다.
보드 내 다른 담당자의 이슈, 이슈 본문·댓글·첨부파일은 조회하지 않았다.
처음에는 최근 갱신 20건의 요약·상태·날짜를 확인했다. 이 응답은 다음 페이지가
있어 전체 할당 업무의 총수로 해석하지 않았다.

이어 세 조건을 각각 최대 50건, 한 페이지로 조회했다. 세 응답 모두 HTTP 200,
isLast=true, 다음 토큰 없음이었다. 조건별 조회는 서로 겹칠 수 있으며 합산하지 않는다.

| 조건 | 관찰 |
|---|---|
| 본인 할당 + 실제 TODO status `10084` | 23건. 기한 없는 이슈 22건, 기한이 지난 이슈 1건 |
| 본인 할당 + statusCategory != Done + 기한 2026-09-07 이상, 2026-09-21 이하 | 0건 |
| 본인 할당 + statusCategory != Done + 기한 2026-09-07 미만 | 10건 |

날짜 범위는 이번 진단을 위해 한국 날짜 기준 오늘부터 14일 뒤까지 양끝 포함으로
명시했다. 확정된 보고 정책은 아니다. 기한 경과는 Jira에 기록된 날짜·상태의
관찰이며, 실제 업무 지연이나 미완료가 독립적으로 검증됐다는 의미는 아니다.
기한이 없으므로 ‘2주 내 업무가 없다’고 결론 내릴 수 없다.

## 검증 한계와 다음 작업

- GET만 사용했고 redirect를 거부했다. 요청별 timeout과 응답 크기 상한을 적용했다.
  저장 토큰을 사용하는 요청은 확인된 Atlassian API origin으로 제한했다.
- 실제 token의 scope 목록과 만료일은 API로 검증하지 않았다. 만료일
  2027-09-07은 사용자 제공 정보다.
- 직접 수동 조회만 수행했다. 앱의 연결 프로필·키체인 import·HTTP reader·live
  acquisition/resume·정규화 증거 보존은 아직 구현·검증되지 않았다.
- 원문 응답을 acquisition으로 보존하지 않았고 새 AI report job·평가·스케줄을
  실행하지 않았다. 장애·rate limit·장기 재인증 검증도 하지 않았다.
- TODO의 날짜 누락 처리, 기한 경과 업무 포함 여부, 본인 할당 조건과 TODO/날짜
  조건의 AND/OR 관계는 아직 보고 정책으로 확정하지 않았다.
- 이번 변경은 검증 문서뿐이다. Go 빌드·테스트를 다시 실행하지 않았다.

[연결 준비서](../setup/JIRA_CONNECTION_PREPARATION.md),
[저장 수집 검증](PHASE_06_JIRA_INGESTION.md),
[Phase 6](../implementation/PHASE_06_JIRA.md).
