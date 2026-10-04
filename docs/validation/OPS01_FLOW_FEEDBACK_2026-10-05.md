# OPS-01 실제 실행 검증 — 2026-10-04/05 KST

[구현 계약](../implementation/OPS01_FLOW_FEEDBACK_2026-10-05.md) · [운영 절차](../setup/FLOW_FEEDBACK.md)

별도 disposable home에서 saved synthetic mail만 사용했다. 기존 ChatGPT 로그인 CLI를
호출했으며 API/key 경로, production 설정·서비스·원문, CLI 설치·다운그레이드나 push는 없다.
가짜 legacy Attempt, 모델 응답, pass judgment나 사람 승인을 넣지 않았다.

## 실행 경계 검사

관찰 executable은 `codex-cli 0.160.0`/Darwin arm64이며 binary SHA-256은
`6b582e8813ce7e8ed4c52814ee5cf230dba647bf2292df747a4003f2657ef201`이다.
reception Sol medium, refinement Luna xhigh, synthesis/review Sol xhigh를 실제 호출했다.
각 실행은 production과 같은 canonical config/sandbox/feature argument 구성을 사용했다.
requested effort를 검증했으며 provider 내부 effective effort를 관찰했다고 주장하지 않는다.

`agents.enabled=false` 없이 시작한 탐색에서는 모델이 collaboration 사용을 자기 보고했으나
그 설명은 신뢰된 도구 실행 증거가 아니었다. 공식 별도 switch를 확인해 적용했다. 그 이후
실제 JSONL에 tool execution 이벤트는 없고 tool host 비활성화가 관찰됐다. 일부 모델의
자기 보고는 여전히 모순적이므로 전체 도구 목록이 신뢰된 방식으로 열거됐다고 주장하지 않는다.

동일 custom permission profile의 실제 OS sandbox 안에서 package 읽기는 성공했고,
sibling 읽기·package 쓰기·sibling 쓰기 시도는 EPERM이었다. 명시적인 IP HTTPS network
시도도 실패했다. interpreter로 시작한 초기 확인은 외부 dylib/Gem 접근이 거부되어 종료됐고,
이것을 원하는 작업의 거부 증거로 대신하지 않았다. system shell/curl로 해당 작업을 직접 시도했다.
Linux/EC2, 다른 executable/version/모델 조합이나 native/MCP task는 검증하지 않았다.

## 실제 메일 baseline/candidate와 독립 평가

같은 objective, 네 개 source, as-of/history, scoped project와 budget을 사용했다.
sources는 체크리스트/차단요인 요청, 별도 교육 견적 요청, 정보성 Jira notice,
source-embedded 가짜 도구 지시다. 모든 주소는 `example.invalid`의 가상 자료다.
두 실행 모두 실제 host collect, Luna refine, Sol synth, host validate/deliver가 완료됐다.
각각 부모 job은 `staged`, 검증 report는 `completed`, 실제 result event는 `available`이다.
외부 송신이 성공했다고 처리하지 않았다.

| 근거 | baseline | candidate |
|---|---|---|
| Job | `K3F5EU4LUQEBQ2FP4PEMVYZMM7` | `NTDUI23M6IFQVA4H3IE4G4NC36` |
| 검증 StepAttempt | `7AJEESYZ2OGKINIZMOEZM5OJDI` | `3KYC4VI3FZIFUUY5FLZM7JHL4E` |
| 독립 evaluation | `NPNERMAVSIPHQ6D5SMZBFBX5BD` | `MGORVKLCDNWZXRZMVHM4ZF22V4` |
| 실제 review outcome | fail | pass |

case `ECXNQFGQ6CTRVXC3ITDQXDPS6Z`, rubric `GBIW6O7PMTBWUO2D4CZO5ZKTTZ`,
evaluator Skill `SN5TUHRTNBECEDOCEG3JEICB2R`는 두 평가에 동일하게 고정했다.
실제 독립 Sol reviewer는 baseline에서 차단요인 목록 요청 누락 및 complete/no-attachment
입력과 맞지 않는 원문/첨부 확인 불가 설명을 지적했다. 완료 미확인은 pass였다.
candidate는 별도 요청과 차단요인 확인, 미확인 완료 및 근거 기준 모두 pass였다.
baseline을 다시 실행해 실패를 지우지 않았다. 한 사례의 차이이며 일반적인 품질 향상이나
인과 관계를 입증하지 않는다.

comparison `TIKDXSLSRB6WMJD4RKXLLNOAKJ`는 comparable=true,
job_count=2, 실제 stage attempt_count=10, failed_or_interrupted_attempts=0,
unevaluated_runs=0이었다. legacy attempts는 만들지 않았고 실제 refinement/synthesis의
requested route/model/effort 및 observed runtime를 비교했다.

실제 usage는 baseline refine input/output 8,468/996, synth 11,022/1,770;
candidate refine 8,468/2,158, synth 11,370/2,346이었다. reviewer 사용량은 이 수치와 별개다.
이 한 사례로 토큰 절감이나 비용 최적화를 주장하지 않는다.

## 차단·복구 확인

- foreground FLOW writer가 실행 중일 때 Skill import는 controller lock으로 거부됐다.
- release assessment `MZ4JPH6QDK3PV2HVKTRN3IDJOE`는 eligible=false였다. validation-authored
  policy/case/rubric, 선언한 평가 한계, 독립 confirmation 및 필수 check 누락을 실제 이유로 남겼다.
- `--actor-kind validation` 채택 시도는 "only an explicit human decision"으로 거부됐고,
  실제 scoped active baseline은 변경하지 않았다.
- 독립 복사본 세 개에서 prompt digest 변경, refinement Skill + 일치하는 audit hash 변경,
  schema + 일치하는 audit hash 변경을 각각 수행했다. 모두 canonical admitted-objective/source
  contract 검사로 거부됐다. 실제 모델 응답/판정은 변경하지 않은 무결성 fault injection이다.
- 별도 복사본의 request revision을 올렸을 때 선택한 과거 evaluation이 거부됐다.
  현재 attempt 하나만 보는 검사가 아니라 quality revision에 평가가 결합됨을 확인했다.
- 실제 candidate reviewer package에는 executor Boundary/ThreadID/전체 Flow와 절대 runtime
  home이 없었고 host ReviewRequest에는 원래 provenance가 유지됐다.
- backup/verify/restore는 성공했다. 94개 파일에 네 개 stage audit, 원래 최종 JSON responses,
  14개 feedback record, scoped component/Bundle/selection이 포함됐다. 별도 restore home에서
  원래 두 실행/평가의 comparison도 성공했다. 운영 연결은 비활성화된 채로 남았다.

## 실행한 코드 검사와 남은 조건

`go test ./internal/executor ./internal/stagedworkflow ./internal/feedback ./internal/runner ./internal/cli`,
같은 직접 영향 범위의 `go vet`, `go build ./cmd/chunsu`를 실행해 통과했다.
새 test source는 작성하지 않았고 full-repository review/test는 하지 않았다.
team-ops의 host-refinement/실제 synthesis/validated-report 연결은 같은 영향 범위에 포함하며,
팀 운영 fixture의 실제 report/reconcile 결과는 별도 OPS-02 검증 기록에 남긴다.
첫 team-ops 실제 partial 보고서는 source timestamp가 raw host에는 있지만 synthesis의
span-only projection에 빠진 문제를 드러냈다. pinned source_index로 시간·channel/thread
metadata를 함께 전달하도록 수정했고 이전 partial 결과를 삭제하거나 완전한 성공으로
바꾸지 않는다. 수정 이후 새 job의 실제 보고서는 OPS-02 기록으로 구분한다.

실제 historical 호환성은 comparison `VGOFCDMDOIAQC3PQPYBBMLXBFB`로 확인했다.
old `4P2LAW4RXI6RDJS4F4TGF3M5NE`와 new `RBFNLLCN3A6SGTZ62YBV26PDEJ`가 모두
InspectRun 검사를 통과했다. old projection version은 빈 historical 값이며 결과는 그대로
partial이다. old output digest도 원래 값으로 유지됐다.

| old artifact | 유지한 SHA-256 |
|---|---|
| raw | `c0d89869c1a3d9e7bc70ac8b0ebe8b937fcf617dd447b56f2de95774fd3f98aa` |
| evidence | `42d16cc8442c6bd9aceed807b8dc83c7a242202d2694b59ae1d92d267adba4c1` |
| synthesis | `a918f5b5f8449124f592357ab55d09c72ada955c0933936b73d4e14a7c82cfdf` |
| validated | `54429ed374daab834fbdd7bf6b05df1dd0ae7ed65b1abeee459a743fdf06400a` |

new host receipt `L5PV4NS5ZMXGV4GHC4ZDPYF4QK`는 `collection_version: ops-source-index-v2`다.
실제 Sol synthesis와 모든 단계가 완료됐고 timestamp/capture context를 읽은 보고서는
기준 시점의 현재 장애 지속 여부를 알 수 없다고 partial을 유지했다. validate anchor는
`NR6V34ZOS5HJQFAOYZH5SIRNT7`, validated artifact는 `O3SGHZBIJQTJPELR2TKMYSPTVV`,
digest는 `7089cc811e3c139e06998b15d2ac83675e6b546547c1cfc908ad8a74e4e4f4de`다.
host reconciliation은 revision 28→29 및 같은 operation의 동일 record replay까지 통과했다.
comparison은 input/목표/host projection version 차이와 semantic 평가 부재로 comparable=false를
남겼다. 새 버전이 old 보고서를 소급 바꾸거나 이 두 다른 입력을 품질 향상 비교로 쓰지 않는다.
team-ops legacy `experiment`는 실제 old job에서 candidate load/Store.Submit 전에 거부됐다.

긍정 채택은 아직 하지 않았다. 사용자의 실제 정책/기대사항 검토, 정당한 독립 확인 사례,
필수 check 및 한계를 다루는 명시적인 판단이 필요하다. 단일 async 정책 승인으로 없는
confirmation을 대신할 수 없다. 운영 품질·노이즈·private 자료 접근·Linux/EC2 경계·외부
알림 송신·실서비스 self-update/deploy는 이 검사의 완료 범위가 아니다.
