# OPS-01 — 현재 Codex CLI와 실제 FLOW 결과 평가

[팀 운영 계획](TEAM_OPERATIONS_2026-10-04.md) · [내부 Skill 계약](SCOPED_SKILLS_2026-10-04.md)

이 단위는 비개발 업무를 실제 AI로 수행하고 결과를 독립 평가할 수 있도록, 설치된
signed-in Codex CLI의 제한된 structured 경로와 FLOW → feedback 연결을 추가한다.
직접 API/키, CLI 설치·다운그레이드, 제품 모델 기본값 변경, 운영 배포는 포함하지 않는다.
개발·독립 코드 리뷰는 GPT-6.1 Sol max이며 제품 reception/refinement/synthesis/review의
기존 Sol/Luna 모델·effort는 유지한다.

## 현재 실행 경계

새로 허용한 조합은 실제 로컬에서 검사한 `codex-cli 0.160.0`, `darwin/arm64`, codex
driver의 structured 호출뿐이다. reception은 `gpt-6-sol`/`medium`, refinement는
`gpt-6-luna`/`xhigh`, synthesis와 review는 `gpt-6-sol`/`xhigh`다. 다른 버전·host·role·
모델·effort를 포괄 승인하지 않는다. 기존 0.158 경로는 유지하며 0.160 native/MCP task
및 Linux/EC2 경계를 검증했다는 뜻이 아니다.

실제 호출은 전용 package 읽기만 허용하는 native-restricted sandbox, 격리된 config,
`--ignore-user-config`, `--ignore-rules`, strict config, 도구 host 및 web/MCP 비활성화를
사용한다. 0.160에는 별도의 `agents.enabled=false`를 추가한다. stream parser는 실제
tool 이벤트가 나타나면 거부한다. 경계 증거는 실제 패키지 읽기 성공 및 sibling 읽기·
쓰기·네트워크 시도 거부, 공식 설정, 실제 역할 호출과 canonical argument hash다.
모델의 도구 자기 보고나 tool 이벤트 부재만을 전체 도구 비공개의 증거로 쓰지 않는다.
provider 추론 통신은 필요하며 tool network 금지와 구분한다.

설정 근거: [비대화형 실행](https://learn.chatgpt.com/docs/non-interactive-mode),
[승인·보안 경계](https://learn.chatgpt.com/docs/agent-approvals-security),
[subagent 전역 설정](https://learn.chatgpt.com/docs/agent-configuration/subagents#global-settings).

## 실제 단계 증거 계약

새 FLOW 결과는 기존 legacy Attempt/Manifest를 만들어 내지 않는다. `RunEvidence.Flow`
는 실제 workflow/step/step_attempt/artifact의 projection이며 `selected_attempt_id`와
`Evaluation.attempt_id`는 실제 validate StepAttempt다. 결과 artifact는 같은 attempt의
`staged_validated`다. synthesis/refinement는 각각 별도 실행·응답 증거를 보존한다.

검사하는 내용은 다음과 같다.

- workflow version, request revision, 원래 objective, 입력 hash, mode, scope, budget,
  완료 기준과 정확한 collect → refine → synthesize → validate ancestry.
- 현재 완료 attempt와 step pin/출력의 일치, source coverage, 원문 excerpt,
  원래 선택 Skill과 실제 적용한 transport adapter, schema와 domain validator.
- 실제 model receipt, requested route/model/effort, 관찰 runtime version, canonical args,
  reply hash와 전용 package의 Skill/schema. prompt/schema/role Skill도 실제 실행과 공유한
  canonical 준비 함수로 재구성한다. 서로 맞춘 임의 audit hash만으로 통과하지 않는다.
- 실패·재시도 attempt를 포함한 quality revision digest. 평가와 comparison은 정확한
  revision을 고정하며 assess도 현재 단계/산출물을 다시 읽는다.

`staged_model_response`는 실제 최종 JSON만 보존한다. 숨겨진 추론·transcript는 저장하지
않으며 기존 protected run storage와 source disclosure 정책, retention/backup을 따른다.
이 증거와 requested selector가 없는 과거 FLOW는 `flow inspect/artifact/result`로 조회할
수 있지만 새 release comparison의 실행 증거로 소급 인정하지 않는다. 저장 입력을 다시
실행해야 한다. DB schema 5와 기존 record schema의 optional field를 사용하며 migration은 없다.

메일은 실제 Luna refinement와 Sol synthesis를 사용한다. `team-ops`는 host가 원래 span을
그대로 투영하고 `staged_host_refinement`라는 별도 typed receipt를 남긴다. host 단계에는
model/effort나 AI receipt를 붙이지 않는다. host projection을 재구성하고 실제 Sol synthesis
receipt를 검증한다. 비교는 같은 refinement engine/버전끼리만 허용한다. team-ops AI 입력은
host가 ledger와 digest-checked raw observation으로 생성한 synthetic 입력만 받는다.
serialized `synthetic` 값 변경만으로 private source를 허용하지 않는다.
원래 source 시각과 수집 시각, source kind/version/channel/thread 등은 pinned `source_index`로
전달하고 span 본문과 구분한다. raw host에 시각이 있다는 이유만으로 모델에도 전달됐다고
가정하지 않으며, 실제 source timestamp와 수집 freshness는 서로 다른 기준이다.
새 projection은 `ops-source-index-v2`로 raw/evidence/host receipt에 고정한다. 이전
span-only projection은 보존된 metadata와 원래 prompt를 그대로 검증해 조회할 수 있지만,
새 버전과 같은 refinement 계약의 comparison으로 취급하지 않는다.

## 평가·비교·채택

독립 reviewer는 pinned case/rubric/evaluator Skill, 허용된 원래 입력, 실제 보고서를 읽고
tool-free 모델로 판단한다. host ReviewRequest는 전체 증거를 보존하지만 reviewer의 model
projection에서 Skill scope/origin 및 실행 boundary의 절대 경로·provider thread ID를 제거한다.
원문 메일의 업무 thread ID와 필요한 업무 사실은 허용된 source 자료로서 별개다.

FLOW와 legacy 실행은 서로 비교하지 않는다. 같은 engine에서도 원래 입력/as-of/history,
objective, scope, mode/budget, workflow contract와 실제 refinement/synthesis requested route,
effort/runtime가 같아야 한다. `Case.ops_snapshot`은 compiled team-ops의 typed input이며
기존 mail/Jira/code case 표현을 변경하지 않는다.

완료된 domain 보고서의 상태와 사용자 전달 상태는 분리한다. 부모 job이 `staged`이고
result event가 `available`이어도 검증 보고서는 `completed`일 수 있다. release의
`allowed_job_statuses`는 FLOW에 대해 이 검증 보고서 상태를 확인한다. 전달 상태 변경은
quality revision에 넣지 않으며 실제 송신 성공을 꾸며 내거나 자동 resolve하지 않는다.

기존 proposal/compare/release-policy/check/owner decision 경로를 재사용한다. stale base,
평가 한계, reviewed expectations, 독립 confirmation, 필수 검사, 회귀와 명시적인 사람
승인 요구를 낮추지 않는다. AI 작성 자료를 `human_reviewed`로 바꾸거나 사용자의 단일
정책 승인만으로 별도 confirmation을 만들어 내지 않는다. 채택/rollback은 owner 작업이다.

[실행 안내](../setup/FLOW_FEEDBACK.md) · [실제 검사와 남은 조건](../validation/OPS01_FLOW_FEEDBACK_2026-10-05.md)
