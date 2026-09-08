# Jira 보고서와 Skill 실행

메일과 Jira는 같은 작업 큐, 실행기, 원문 게이트웨이, 결과 보존 및 평가 경로를
사용한다. Jira 수집기는 회사 API를 읽고, 실행기는 보존된 입력만 읽는다.

## 관리하는 지침

| 원본 | 역할 |
|---|---|
| `workgroups/mail-review/SKILL.md` | 메일 보고서 작성 |
| `workgroups/jira-report/SKILL.md` | Jira 보고서 작성 |
| `evaluation-skills/golden-evaluation/SKILL.md` | 독립 결과 평가 |
| 각 workgroup의 `report.schema.json` | 공개 결과 계약 |

새 실행은 선택한 Skill의 이름·설명·해시와 작업 종류를 manifest에 기록한다.
실행 패키지에는 `SKILL.md`, 동일 내용을 직접 주입한 `instructions.md`,
출력 스키마와 원문 목록이 들어간다. 실행 직전에 파일과 선택된 Skill의
일치 여부를 검사한다. 전역 Skill 검색에 의존하지 않는다.

기존 v1 메일 Guide는 저장된 버전과 해시를 바꾸지 않고 새 실행에서 Skill로
감싼다. 평가 Skill과 기대 답안은 보고서 생성 패키지에 들어가지 않는다.

## 합성 입력 실행

```sh
chunsu setup
chunsu queue examples/jira/report-input.json --workgroup jira-report
chunsu run JOB_ID
```

`--workgroup`을 생략한 기존 `queue` 명령은 메일을 처리한다. Jira 원문 조회는
해당 시도의 `jira_issue_get`만 제공하며, 모든 선택 이슈의 조회를 확인한다.

## 실제 Jira 연결

`jira connect --help`에서 회사 사이트·Cloud ID·본인 계정·프로젝트·보드·
TODO 상태·날짜 필드·시간대와 기존 Keychain 참조를 지정한다. 현재 연결은
개인 Atlassian API 토큰과 계정 이메일을 사용하는 읽기 전용 Cloud 연결이다.
토큰 값은 설정 파일이나 실행 패키지에 저장하지 않는다.

연결은 본인 계정, 보드와 프로젝트, 선택한 TODO 상태의 보드 소속과 상태 분류,
날짜 필드를 확인한다. 수집 시 기준일·조회 범위·정책을 고정하고, 재개와
보고서 생성에서도 같은 정책인지 확인한다.

```sh
chunsu jira collect-live CONNECTION_ID
chunsu jira queue ACQUISITION_ID
chunsu run JOB_ID
```

실제 데이터의 실행기 전달에는 완료된 합성 Jira 검증 작업과 현재 정책 해시를
지정한다. 실행기 종류·경로·모델을 바꾸면 승인이 해제된다.

```sh
chunsu config set executor.live_jira_validation_job_id SYNTHETIC_JOB_ID
chunsu config set executor.live_jira_policy_digest POLICY_DIGEST
chunsu config set executor.live_jira_approved true
```

`POLICY_DIGEST`에는 해당 작업의 보존된 입력에 있는 `report_policy_digest`를
사용한다. 수집 정책의 해시와는 다른 값이다.

검증 작업은 실제 실행에 선택할 Jira Skill 버전과 같은 실행기를 사용하고, 한 개 이상의 합성
원문을 모두 조회하여 정상 완료되어야 한다. 실제 실행 직전에도 이를 다시
확인한다. 메일 승인은 Jira 승인으로 사용되지 않는다.

## 독립 평가와 개선 후보

```sh
chunsu feedback pin-evaluator evaluation-skills/golden-evaluation/SKILL.md --workgroup jira-report
chunsu feedback import rubric RUBRIC.json
chunsu feedback import case CASE.json
chunsu feedback import evaluation EVALUATION.json --job JOB_ID
chunsu workgroup show --workgroup jira-report
chunsu workgroup propose --workgroup jira-report --help
chunsu experiment JOB_ID --candidate CANDIDATE_DIGEST
```

Jira 사례는 `workgroup: "jira-report"`와 전체 입력을 담은 `jira_snapshot`을
사용한다. 평가에는 고정한 평가 Skill의 기록 ID와 해시가 필요하다.
`pin-evaluator`는 평가 지침을 보존하는 명령이며 평가 모델을 자동 호출하지
않는다. 별도 평가자가 작성한 판정을 가져오는 방식이다.

실제 데이터로 후보를 실행하려면 같은 후보 버전으로 합성 검증을 먼저
완료하고, 그 작업을 `executor.live_jira_validation_job_id`로 지정한다.
검증은 활성 버전 또는 해당 후보 버전에만 적용되며 다른 Skill이 이어받지 않는다.
검증 작업을 바꾸면 실제 데이터 전달 승인이 해제되므로, 승인된 범위에서
`executor.live_jira_approved`를 마지막에 설정한다.

후보 실행과 비교는 활성 Skill을 변경하지 않는다. 채택은 별도의 명시적
결정이다. 일정은 기존 경과 시간 간격 방식이며 처음에는 비활성으로 생성된다.
백업 복원 후에는 연결·실행 승인·일정이 비활성 상태가 된다.

인증 방식의 근거: [Atlassian 개인 API 토큰 문서](https://support.atlassian.com/atlassian-account/docs/manage-api-tokens-for-your-atlassian-account).
