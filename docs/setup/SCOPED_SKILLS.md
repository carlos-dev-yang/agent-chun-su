# 내부 Skill 범위와 버전 관리

[구현 계약](../implementation/SCOPED_SKILLS_2026-10-04.md) · [검증 기록](../validation/TEAM_08A_SCOPED_SKILLS_2026-10-04.md)

같은 owner의 데이터 저장소에 버전을 중앙 보관하고, 공통/팀/프로젝트별로 정확한 조합을
선택한다. 팀 ID는 조직용 namespace이며 사용자 인증·프로젝트 권한 검사는 아직 없다.
아래 `pilot`/`service-a`는 예시 이름이며 실제 데이터·배포 설정이 아니다.

모든 명령에서 동일한 `--home PRIVATE_DATA_HOME`을 사용하거나 `CHUNSU_HOME`을 지정한다.
관리 변경은 controller/worker를 정지한 유지보수 상태에서 실행한다. `pause`만으로
controller writer lock이 해제되지는 않는다. read-only `skills active/list/show/preview`는
운영 중에도 읽을 수 있다. 최초 설치는 기존 setup 절차를 따른다.

## 범위 초기화와 지침 등록

```sh
chunsu --home PRIVATE_DATA_HOME skills initialize --workgroup mail-review \
  --skill-scope project --team pilot --project service-a \
  --actor OWNER --reason '기존 domain 기준을 프로젝트 baseline으로 고정'
```

이미 선택된 domain 기준을 복사하며 기존 범위를 덮어쓰지 않는다. 기준 Skill/schema의
내용은 바꾸지 않는다. 팀 범위는 `--skill-scope team --team pilot`, 공통은
`--skill-scope common`으로 지정한다. 다른 범위에서 쓸 구성요소를 등록하려고 해당 범위를
모두 초기화할 필요는 없다. 실행/채택할 목표 범위만 초기화하면 된다.

다음은 비활성 공통 instruction JSON 예다. 파일은 편집기로 준비한다.

```json
{
  "version": 1,
  "name": "completion-evidence",
  "scope": {"kind": "common"},
  "purpose": "완료 판단의 근거를 확인",
  "instructions": "완료 주장에는 확인 가능한 근거를 연결하고 근거가 없으면 불확실성을 표시한다.",
  "rules": [{"key": "review.completion", "value": "observable-evidence-required"}]
}
```

```sh
chunsu --home PRIVATE_DATA_HOME skills import COMMON.json \
  --actor OWNER --reason '재사용 가능한 판단 지침' --sanitized-for-scope
chunsu --home PRIVATE_DATA_HOME skills import PROJECT.json \
  --actor OWNER --reason '프로젝트 피드백을 반영' --evidence FEEDBACK_RECORD_ID
chunsu --home PRIVATE_DATA_HOME skills show SKILL_DIGEST
chunsu --home PRIVATE_DATA_HOME skills list
```

팀 scope JSON은 `{"kind":"team","team":"pilot"}`, 프로젝트는
`{"kind":"project","team":"pilot","project":"service-a"}`다. 응답의 `skill_digest`가
정확한 버전이다. 동일 이름의 새 버전도 별도 digest로 등록되며 기존 실행은 바뀌지 않는다.
민감정보·비밀값·원문 작업 기록은 지침에 넣지 않는다. 공통 공개 시 선언은 owner의
확인 기록이며 자동 마스킹 증거가 아니다.

## 명시적 조합과 후보 비교

`COMPOSITION.json`은 구성요소의 **전체** digest 목록이다. 추가/교체할 때도 전체 목록을
다시 명시한다. 최신 버전 탐색이나 자동 상속은 없다.

```json
{
  "version": 1,
  "workgroup": "mail-review",
  "scope": {"kind": "project", "team": "pilot", "project": "service-a"},
  "components": ["COMMON_SKILL_SHA256", "TEAM_SKILL_SHA256", "PROJECT_SKILL_SHA256"]
}
```

```sh
chunsu --home PRIVATE_DATA_HOME skills preview COMPOSITION.json
chunsu --home PRIVATE_DATA_HOME skills propose COMPOSITION.json \
  --evidence FEEDBACK_RECORD_ID --hypothesis '완료 근거 누락을 줄임' \
  --check grounding-and-regression
chunsu --home PRIVATE_DATA_HOME workgroup diff PROPOSAL_ID
chunsu --home PRIVATE_DATA_HOME queue SAVED_INPUT.json --workgroup mail-review \
  --skill-scope project --team pilot --project service-a
chunsu --home PRIVATE_DATA_HOME run BASELINE_JOB_ID
chunsu --home PRIVATE_DATA_HOME experiment BASELINE_JOB_ID --candidate CANDIDATE_BUNDLE_SHA256
chunsu --home PRIVATE_DATA_HOME run CANDIDATE_JOB_ID
```

후보 Bundle digest는 `workgroup diff`의 `proposal.candidate_digest`다. 코드상 충돌하는
`review.*` 기준은 거부한다. 자연어 충돌, 누락, 노이즈·비용·독립 사례는 preview와
결과 검토에서 확인한다. release-policy와 evaluator 준비는 기존
[결과 리뷰 안내](RESULT_REVIEW.md)를 따른다.

```sh
chunsu --home PRIVATE_DATA_HOME feedback compare BASELINE_JOB_ID CANDIDATE_JOB_ID \
  --baseline-evaluation BASELINE_EVALUATION_ID --candidate-evaluation CANDIDATE_EVALUATION_ID
chunsu --home PRIVATE_DATA_HOME workgroup assess PROPOSAL_ID \
  --comparison COMPARISON_ID --policy RELEASE_POLICY_ID --check-result CHECK_RESULT_ID
chunsu --home PRIVATE_DATA_HOME workgroup decide PROPOSAL_ID --action adopt \
  --actor OWNER --reason '검증 결과와 남은 한계 확인' \
  --comparison COMPARISON_ID --policy RELEASE_POLICY_ID --check-result CHECK_RESULT_ID
chunsu --home PRIVATE_DATA_HOME skills active --workgroup mail-review \
  --skill-scope project --team pilot --project service-a
chunsu --home PRIVATE_DATA_HOME workgroup decide PROPOSAL_ID --action rollback \
  --actor OWNER --reason '이전 기준으로 복귀'
```

평가 누락/비교 불가/필수 검사 누락/기준 변경이면 채택하지 않는다. 공통·팀·프로젝트
선택은 독립적이다. 공통 후보를 채택해도 프로젝트가 자동으로 새 버전을 사용하지 않는다.
새 공통 버전을 프로젝트 조합에 명시하고 그 프로젝트에서도 검증·채택한다.

## FLOW 실행과 피드백

```sh
chunsu --home PRIVATE_DATA_HOME flow submit mail-review SAVED_INPUT.json \
  --objective '근거를 확인해 업무를 검토' --checkpoint \
  --skill-scope project --team pilot --project service-a
chunsu --home PRIVATE_DATA_HOME flow inspect JOB_ID
chunsu --home PRIVATE_DATA_HOME flow run JOB_ID
chunsu --home PRIVATE_DATA_HOME skills feedback JOB_ID --kind insufficient-evidence \
  --observation '완료 근거가 부족했음' --correction '필요한 원문 근거와 교정 내용을 기록' --actor OWNER
chunsu --home PRIVATE_DATA_HOME skills list --kind skill_feedback
```

비활성 조합 실행에는 `queue` 또는 `flow submit`에 `--skill-candidate BUNDLE_SHA256`을
추가한다. 기존 scoped job의 pin을 바꾸는 것은 아니며 새 job을 만든다. Scope가 다른
후보는 거부한다. checkpoint의 다음 단계는 기존 `flow approve STEP_ID`로 승인한다.
새 exact response/audit 증거를 보존한 FLOW는 [독립 평가·후보 비교](FLOW_FEEDBACK.md)로
기존 release gate에 연결할 수 있다. 증거가 없는 과거 FLOW와 legacy 실행을 섞지 않는다.
0.160 structured compatibility를 쓸 때는 해당 안내의 정확한 host/role 제한을 확인한다.

피드백 kind는 `false-positive`, `omission`, `severity`, `owner`, `duplicate`,
`insufficient-evidence`, `useful`이다. `useful` 외에는 교정 내용이 필요하다.
등록 근거는 실행 전/중간 단계 피드백일 수도 있으므로 완료 품질 판단으로 간주하지 않는다.
피드백 → 정제된 지침 JSON → 비활성 후보 → 비교 → 사람 채택 순서이며 자동 학습/활성화는 없다.

권한·연결·모델 route 변경 검사는 유지한다. 현재 installed Codex 버전/모델의 실행 경계가
검증되지 않으면 실제 AI 단계는 막힌다. Skill 채택으로 그 검사를 우회할 수 없다.
`backup`/`verify-backup`/`restore`는 기존 명령을 쓰며 scoped 자산도 포함된다.
restore 후에는 source disclosure와 queue 상태를 별도로 검토한다.
