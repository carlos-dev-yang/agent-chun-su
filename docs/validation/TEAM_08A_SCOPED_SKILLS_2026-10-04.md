# TEAM-08A — scoped internal Skill 검증

2026-10-04 KST. [구현 계약](../implementation/SCOPED_SKILLS_2026-10-04.md) ·
[owner 사용 안내](../setup/SCOPED_SKILLS.md).

상태: 로컬 관리/실행 pin 통합 구현 및 독립 코드 리뷰 완료. 실제 모델 결과의 품질,
사내 소스, 팀 사용자 권한, Slack·외부 알림, 운영 서비스 활성화·push는 검증하지 않았다.
새 테스트 소스는 작성하지 않았다.

## 실제 실행한 검사

- 변경 범위의 기존 `go test`와 `go vet`: `internal/workgroup`, `skills`, `feedback`,
  `store`, `control`, `stagedworkflow`, `runner`, `cli`, `executor` 통과.
- `go build ./cmd/chunsu`로 macOS arm64 disposable binary를 만들고 Linux amd64
  cross-build도 통과했다. Linux binary를 실제 서버에서 실행한 것은 아니다.
- GPT-6.1 Sol max 독립 리뷰는 기존 gate/권한 경계와 새 scoped 실행/CLI를 확인했다.
  두 발견(전역 proposal이 scoped 비교를 인정하는 비대칭 조건, 조합의 선언된 원본 hash를
  실제 plain Bundle에 연결하지 않는 문제)을 수정하고 해당 delta를 재검토했다.
  남은 코드 발견은 없었다. scope labels는 RBAC가 아니라는 한계와 FLOW 비교 제약은 유지한다.
- `git diff --check`와 신규 문서의 상대 링크 확인을 수행했다.

## Disposable CLI/host 시나리오

격리 위치는 owner가 만든 private `.codex/chunsu-team08a.*` 아래다. 실계정 자격증명이나
기존 운영 data home을 사용하지 않았다. setup 결과는 schema 5이며 migration 파일은 없다.

| 시나리오 | 실제 관측 |
|---|---|
| 비활성 import | 공통/팀/프로젝트와 충돌용 fixture를 immutable digest로 등록; active baseline 불변 |
| 공통 공개 선언 | `--sanitized-for-scope`가 없으면 거부; 선언은 자동 비식별화 증거가 아님 |
| baseline 초기화 | 기존 선택 domain Bundle만 빈 scope에 기록; 같은 scope 재초기화 거부 |
| 팀/프로젝트 관계 | 동일 project 이름의 다른 팀은 별도 pointer; 첫 팀 구성요소를 다른 팀에 조합하면 거부 |
| 구조화 기준 충돌 | 동일 `review.completion`의 다른 값을 거부; 조합은 저장/활성화되지 않음 |
| 명시적 조합 | 세 구성요소를 정확한 digest로 조합, inactive proposal 생성 |
| queue/experiment pin | baseline/candidate request와 실제 package에 각각 정확한 Bundle/Skill hash가 보존됨 |
| executor projection | 실제 생성된 composed `SKILL.md`에 세 instruction section 존재; task package에는 host team/project/actor/reason/evidence sentinel 없음 |
| FLOW host 실행 | scoped candidate의 실제 host collect 완료, 다음 refine은 owner checkpoint에서 대기 |
| scoped 피드백 | 원래 scope/Bundle/input/artifact hash를 연결; active pointer 변경 없음 |
| 원본 integrity | outer digest와 text가 자기일관적인 조합이라도 존재하지 않는 원본 Bundle을 선언하면 admission 거부 |
| backup/restore | 기존 backup/verify-backup/restore 성공; 구성요소·scope 선택·등록/피드백·조합·작업이 보존됨 |

현재 설치 `codex-cli 0.160.0`은 기존 task의 `0.153.4` pin이나 staged structured의
기존 host-specific 검증에 해당하지 않는다. 두 legacy 실행은 package를 정상 생성한 뒤
기존 실행기 버전 gate에서 `waiting_input`으로 거부됐다. 실제 모델 호출·최종 report 성공을
주장하지 않으며 같은 version 거부를 더 재시도하거나 호환성 gate를 확장하지 않았다.
FLOW는 host collect/checkpoint까지만 실제 실행했다.

## 합성 release-gate 검증과 한계

원래 버전 거부 결과는 그대로 보존했다. backup을 별도 data home으로 restore한 **복사본**에서
SQL/immutable JSON fault fixture로 두 report/완료 attempt/provider outcome과 validation
judgment를 심었다. 이 결과는 모델 산출물이나 실제 품질 판정이 아니다. baseline/candidate
manifest는 실제 CLI가 만든 정확한 scoped package를 사용했다. 합성 evaluator/case/rubric,
policy/check를 기존 import와 comparison API로 읽혀 선택 경로를 확인했다.

- 동일 입력·route·scope의 정확한 두 Bundle 비교가 comparable로 기록됐다.
- 필수 check 결과 누락은 eligible=false, `actor-kind validation`의 채택은 거부됐다.
- fixture owner 결정은 해당 project pointer만 변경하고 기존 global pointer는 유지했다.
- 같은 proposal의 stale baseline 채택은 거부됐다. rollback은 원래 baseline으로 복귀했다.
- 별도 global proposal fixture는 같은 digest의 scoped 비교를 release 증거로 인정하지 않았다.
- composed 버전을 활성 중 새 FLOW를 admit한 뒤 rollback했다. 그 job의 원래 composed pin으로
  실제 host collect가 완료되고 checkpoint에 대기했다. 현재 active pointer와 admission pin이
  달라도 old scoped job이 유지되는 경로를 확인했다.
- 별도 프로세스의 inspect/active/recover가 이전 pin/선택을 유지했다. recover 결과는
  interrupted=0이며 실제 중단된 AI 프로세스의 복구 실험은 아니다.

이 증거는 scoped gate와 선택/복구의 기술적 작동만 보여 준다. 사람이 검토한 실업무 개선,
긴급 누락/오탐/노이즈/비용 개선 또는 독립 confirmation 품질을 입증하지 않는다.

## 다음 검증/운영 경계

현재 release comparison/evaluator는 legacy report attempt에 대한 것이다. 실제 채택에는
scoped `queue`/`experiment` 결과와 기존 정책·평가·검사 근거를 사용한다. FLOW-only 두 결과를
현재 `feedback compare`에 넘겨 채택할 수 있다고 안내하지 않는다.

현재 installed CLI/모델의 경계 검증은 별도 작업이다. 실제 사내 데이터/팀 접근은 TEAM-03/05,
팀 판단/온콜/파일럿은 TEAM-04/06/09가 필요하다. DB migration은 추가하지 않았지만
기존 schema-4 설치는 이미 별도 FLOW schema-5 전환이 필요하다. TEAM-08 전체의 팀 인증,
프로젝트 접근 격리, 자동 개선 생성, 일반 프로젝트 지식 저장소는 완료로 표시하지 않는다.
