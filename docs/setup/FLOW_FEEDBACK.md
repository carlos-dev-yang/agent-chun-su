# FLOW 결과를 독립 평가하고 후보와 비교하기

[구현 계약](../implementation/OPS01_FLOW_FEEDBACK_2026-10-05.md) · [실제 검증](../validation/OPS01_FLOW_FEEDBACK_2026-10-05.md)

운영 home을 쓰기 전에 별도 private data home과 synthetic saved input에서 검증한다.
아래 대문자는 실제 값으로 바꿀 placeholder다. signed-in Codex CLI만 사용하며 API 키를
추가하거나 CLI를 설치/다운그레이드할 필요는 없다. 현재 새 compatibility는
`codex-cli 0.160.0`/macOS arm64의 특정 structured 역할만 검증했으며 Linux/EC2는 미검증이다.

```sh
chunsu --home PRIVATE_DATA_HOME setup
chunsu --home PRIVATE_DATA_HOME config route task --driver codex \
  --path SIGNED_IN_CODEX_EXECUTABLE --model gpt-6-sol --effort xhigh \
  --environment native-restricted
chunsu --home PRIVATE_DATA_HOME config policy apply
```

기존 role 정책을 적용하는 예이며 제품 기본 모델을 6.1로 바꾸는 명령이 아니다.
설정된 executable의 `--version`과 `login status`를 확인한다. 이 설정으로 0.160 legacy
`queue/run` native task까지 허용되는 것은 아니므로 아래 saved FLOW 경로를 사용한다.
controller가 writer lock을 갖고 있으면 별도 관리 writer를 열지 않는다. 긴 foreground
`flow run`/`review evaluate`가 끝난 다음 다음 관리 mutation을 실행한다.

## 같은 입력·목표의 두 실행

[범위별 Skill 안내](SCOPED_SKILLS.md)에 따라 scope baseline을 초기화하고 비활성 후보
proposal을 만든다. `CANDIDATE_BUNDLE_SHA256`은 proposal ID가 아니라 Bundle digest다.
그다음 동일 파일과 objective로 두 번 admit한다. 기존 job의 pin을 수정하지 않는다.

```sh
chunsu --home PRIVATE_DATA_HOME flow submit mail-review SAVED_INPUT.json \
  --objective '저장된 메일의 요청, 일정과 완료 근거를 검토' \
  --skill-scope project --team pilot --project service-a
chunsu --home PRIVATE_DATA_HOME flow run BASELINE_JOB_ID
chunsu --home PRIVATE_DATA_HOME flow submit mail-review SAVED_INPUT.json \
  --objective '저장된 메일의 요청, 일정과 완료 근거를 검토' \
  --skill-scope project --team pilot --project service-a \
  --skill-candidate CANDIDATE_BUNDLE_SHA256
chunsu --home PRIVATE_DATA_HOME flow run CANDIDATE_JOB_ID
chunsu --home PRIVATE_DATA_HOME flow inspect BASELINE_JOB_ID
```

`--checkpoint`를 넣었다면 후속 단계마다 `flow approve STEP_ID`로 owner가 승인한다.
`flow inspect`의 result event와 `flow result EVENT_ID`로 실제 검증 보고서를 읽는다.
조회나 품질 평가가 외부 전달 성공을 기록하는 것은 아니다.

## 원래 case와 독립 reviewer 준비

case는 정확한 원래 input과 필요한/금지된/허용 대안 기대사항을 담는다. mail은 `snapshot`,
Jira는 `jira_snapshot`, code는 `code_snapshot`, team-ops는 `ops_snapshot`을 사용한다.
한 case에는 해당 compiled domain의 typed input만 넣는다. 기대사항과 rubric은 결과를
통과시키기 위해 수정하지 않는다. 검증 작성 자료는 `review_status: validation`으로 남긴다.
실제 사람이 검토한 내용과 독립 확인 사례가 있을 때만 그 출처를 기록한다.
team-ops는 legacy `experiment`를 쓰지 않는다. 같은 ledger/as-of/objective에서
`ops report` 두 개를 만들고 후보에 `--skill-candidate BUNDLE_SHA256`을 명시한다.
ledger가 바뀐 두 보고서는 같은 input으로 비교할 수 없으며 다른 Skill scope의 후보도 거부한다.

```sh
chunsu --home PRIVATE_DATA_HOME feedback import case CASE.json
chunsu --home PRIVATE_DATA_HOME feedback import rubric RUBRIC.json
chunsu --home PRIVATE_DATA_HOME feedback pin-evaluator EVALUATOR_SKILL.md \
  --name EVALUATOR_NAME --workgroup mail-review
chunsu --home PRIVATE_DATA_HOME review evaluate BASELINE_JOB_ID \
  --case CASE_ID --rubric RUBRIC_ID --evaluator-skill EVALUATOR_SKILL_ID
chunsu --home PRIVATE_DATA_HOME review evaluate CANDIDATE_JOB_ID \
  --case CASE_ID --rubric RUBRIC_ID --evaluator-skill EVALUATOR_SKILL_ID
chunsu --home PRIVATE_DATA_HOME feedback compare BASELINE_JOB_ID CANDIDATE_JOB_ID \
  --baseline-evaluation BASELINE_EVALUATION_ID --candidate-evaluation CANDIDATE_EVALUATION_ID
```

여러 평가가 있으면 ID를 명시한다. evaluator Skill pin 자체는 모델을 실행하지 않는다.
`review evaluate`가 실제 independent review 실행과 host receipt를 남긴다. 실패 결과,
unknown judgment와 synthetic/coverage 한계를 지우거나 사람이 한 평가로 바꾸지 않는다.
과거 FLOW에 exact response/requested-selector 증거가 없으면 조회만 가능하며 comparison에
쓰려면 재실행해야 한다. 평가 후 request revision/현재 attempt/산출물이 바뀌면 새 평가·비교가 필요하다.

## release와 사람이 선택하는 활성 버전

[기존 결과 리뷰 안내](RESULT_REVIEW.md)의 versioned release policy 및 check 결과를 준비한다.
정책 승인, confirmation provenance와 필수 check는 서로 다른 증거다. 단일 synthetic 사례의
AI pass는 운영 채택 승인이 아니다.

```sh
chunsu --home PRIVATE_DATA_HOME workgroup assess PROPOSAL_ID \
  --comparison COMPARISON_ID --policy RELEASE_POLICY_ID --check-result CHECK_RESULT_ID
chunsu --home PRIVATE_DATA_HOME workgroup decide PROPOSAL_ID --action adopt \
  --actor OWNER --reason '사람이 실제 증거와 남은 한계를 검토함' \
  --comparison COMPARISON_ID --policy RELEASE_POLICY_ID --check-result CHECK_RESULT_ID
```

eligible이 아니면 조건을 읽고 현재 baseline을 유지한다. synthetic acceptance를 위해
human review나 독립 확인을 임의 생성하지 않는다. scope의 active pointer가 바뀌어도 이미
admit한 scoped FLOW는 원래 Bundle을 쓴다. live disclosure/route/연결 철회 검사는 계속 유효하다.

backup에는 실제 최종 JSON response, 단계 audit, 평가·비교 및 scoped 구성요소가 포함된다.
기존 `backup`, `verify-backup`, `restore`로 검증한다. restore 후 연결/공개 권한은 다시
검토하며 기존 중단된 전달 상태를 실제 성공으로 처리하지 않는다.
