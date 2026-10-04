# TEAM-08A — 공통·팀·프로젝트 내부 Skill 관리

[상위 계획](../../IMPLEMENTATION_PLAN.md) · [팀 운영 방향](TEAM_OPERATIONS_2026-10-04.md)

사용자가 2026-10-04 승인한 범위는 중앙 버전 저장소, 공통/팀/프로젝트 적용 범위,
명시적 버전 조합, 피드백 출처, 비교 후 사람의 채택/되돌리기다. 개발과 독립 리뷰는
GPT-6.1 Sol max를 사용하며 메인 에이전트는 범위·증거·진행을 관리한다.

## 이번 구현 계약

현재 단일 OS 소유자와 private data home에서 관리한다. `scope`는 재사용과 선택의
namespace이며 팀 사용자를 인증하거나 데이터 접근을 격리하지 않는다. Slack 온콜,
다중 사용자 RBAC, 내부 업무 상태/배포 모델, 자동 AI 개선 생성은 이 작업에 포함하지 않는다.

| 자산 | 의미 / 저장 |
|---|---|
| Scoped Skill | 목적·업무 판단 지침·선언적 `review.*` 기준; `workgroups/skills/versions/<sha256>.json` |
| 등록/피드백 | 변경 이유·행위자·근거 ID, 실행 입력/산출물 hash; 기존 immutable `records` |
| 조합 Bundle | 기존 compiled workgroup의 Skill/schema와 구성요소의 정확한 snapshot; 기존 workgroup 버전 저장소 |
| 범위별 선택 | scope의 canonical JSON hash + workgroup별 active pointer; 다른 범위에는 전파되지 않음 |
| 실행 pin | queue request / FLOW source_scope의 범위와 Bundle digest; 실제 Skill digest는 기존 실행 증거에 보존 |

Scope는 `common`, `team + team ID`, `project + team ID + project ID`다.
프로젝트 이름이 같아도 소속 팀이 다르면 다른 namespace다. 이름은 소문자 영문·숫자·`-`·`_`
문자만 허용한다. 공통 지침은 모든 범위에, 팀 지침은 같은 팀과 그 팀 프로젝트에,
프로젝트 지침은 정확히 같은 팀/프로젝트에만 조합할 수 있다.

작업 사실·원문 기록은 실행 증거에, 현재 프로젝트 사실은 별도의 지식/설정에 두고,
반복 가능한 수행 방법만 Skill로 등록한다. 이번에는 일반적인 프로젝트 지식 저장소를
추가하지 않았다. `skills feedback`의 분류는 오탐/누락/등급/담당자/중복/근거 부족/유용함이다.
피드백 자체가 활성 지침을 변경하지 않는다.

## 조합과 권한

자동 상속이나 최신 버전 자동 선택은 없다. 구성요소 digest의 전체 목록을 명시한다.
동일 scope/name의 여러 버전은 함께 넣을 수 없다. 같은 `review.*` 키의 다른 값은
구성을 거부한다. 동일한 값은 유지하며 좁은 범위가 넓은 범위 값을 덮어쓰지 않는다.
자연어 문장의 의미 충돌을 전부 탐지하는 기능은 없으므로 preview/diff로 사람이 검토한다.
실행 지침에도 충돌 발견 시 보고하도록 요구한다.

지침은 도구·소스 접근·외부 쓰기·알림 수신자·예산을 변경하지 못한다. `review.*`는
판단 기준이며 host 설정의 실행 규칙으로 해석하지 않는다. 기존 compiled workgroup만
실행할 수 있다. 조합의 원본 Bundle은 실제 immutable plain domain Bundle hash와
일치해야 하며 구성요소 snapshot과 최종 Skill text를 검증한다. 재조합은 원본 domain
Bundle에서 다시 만들고, 이전 구성요소가 숨은 중첩 지침으로 남지 않도록 한다.

원문 피드백, 등록/승인 행위자, 변경 사유, scope ID는 host 관리 기록이다. task package와
reviewer 입력에는 새 scope provenance를 제거하고 선택된 업무 지침만 전달한다.
지침 본문에 사람이 직접 입력한 정보의 적절성은 해당 등록자가 확인해야 한다.
공통 등록은 `--sanitized-for-scope` 선언이 필요하며 실제 비식별화를 자동 보증하지 않는다.
상위 범위에 재사용할 때는 정제된 별도 버전을 등록하고 다시 비교한다.

## 선택·비교·실행

1. `skills initialize`는 이미 선택된 기존 domain 기준만 빈 scope에 고정한다.
   새 지침을 즉시 활성화하는 경로가 아니며 새로운 품질 증거라고 표시하지 않는다.
2. `skills import`는 비활성 instruction asset과 별도 등록 근거를 남긴다.
3. `skills preview/propose`는 명시적 조합을 만들고 기존 proposal을 재사용한다.
4. scoped `queue` baseline과 `experiment --candidate`로 동일 입력의 후보를 실행한다.
   기존 evaluator/comparison/release-policy/check-result가 그대로 필요하다.
5. `workgroup assess/decide`는 해당 scope의 baseline이 그대로인지 확인한다.
   사람의 채택은 정확한 범위의 두 실행, 비교 가능성, 정책과 필수 검사 모두를 요구한다.
   reject는 선택을 바꾸지 않는다. rollback은 현재 활성 후보를 이전 baseline으로 되돌린다.

관리 쓰기는 기존 controller writer lock을 사용한다. 운영 controller가 소유 중일 때
관리 mutation을 별도 writer로 수행하지 않는다. scoped queue와 FLOW admission은 기존
owner management IPC도 사용할 수 있다. reception AI capability에는 scope/후보 선택
기능을 새로 노출하지 않았다.

scoped queue와 FLOW는 admission의 정확한 Bundle을 유지한다. 이후 다른 버전을 채택하거나
되돌려도 admitted job이 새 지침을 몰래 사용하지 않는다. scoped job에서 다른 후보를
실험하려면 새로운 experiment/job을 만든다. 권한 철회·live disclosure·route 변경·mail mode
검사는 계속 유효하다. Jira는 실행 Bundle·route·policy에 맞는 synthetic proof가 필요하며,
FLOW는 application scope와 stage role도 일치해야 한다. 기존 unscoped legacy와 FLOW의
선택/active-match 동작은 유지한다.

FLOW의 collect/refine/synthesize/validate/deliver에는 pin이 통합됐지만, 기존 독립 평가와
release comparison은 legacy report attempt를 읽는다. **FLOW 결과만 두 개 실행해서
채택하는 경로는 이번에 추가하지 않았다.** 후보 채택 증거는 scoped queue/experiment 경로로
만든다. 자동 AI 후보 작성/상위 범위 승격도 아직 없다.

## 저장·검증 경계

DB schema는 5로 유지하고 migration을 추가하지 않는다. 기존 backup의 `workgroups`,
`evaluations`, `proposals`, `runs`에 구성요소·조합·선택·근거가 포함된다. restore의
disclosure 철회/queue pause와 기존 보존 한계는 유지한다. 과거 schema 4 설치는 별도 FLOW
schema-5 전환이 필요하며 이번 변경이 기존 updater의 migration 거부를 해제하지 않는다.

[사용 안내](../setup/SCOPED_SKILLS.md)와 [실제 검증 기록](../validation/TEAM_08A_SCOPED_SKILLS_2026-10-04.md)을
기준으로 상태를 확인한다. TEAM-08A는 로컬 범위 관리의 구현 단위이며 TEAM-08의
팀 권한·실업무 판단·팀 격리·파일럿 전체 완료를 뜻하지 않는다.
