# 로컬 팀 업무·배포·위험 흐름 사용 안내

[설계/계약](../implementation/OPS_LOCAL_2026-10-04.md) ·
[실제 검증과 제한](../validation/OPS_02_LOCAL_2026-10-05.md)

이 기능은 한 owner의 private 데이터 홈에서 팀/project 명칭을 나누는 CLI 파일럿이다.
팀 계정, 팀 권한, Slack 연속 수집이나 실제 메신저 발송은 없다. 업무 사실은 로컬에
저장할 수 있지만, AI 보고는 명시적으로 선언한 공개 합성 scope/입력만 가능하다.
기존 실행 서비스/계정/모델 설정을 예시 때문에 바꾸지 않는다.

## 1. 별도 공개 합성 파일럿 시작

아래 예시는 repository root에서 새 private home을 사용한다. 실제 운영 home을
가리키지 않는다. `OPS_DEMO_HOME`은 이 예시에만 쓰는 task 변수다.

```sh
OPS_DEMO_HOME=$(mktemp -d "${TMPDIR:-/tmp}/chunsu-ops-demo.XXXXXX")
bin/chunsu --home "$OPS_DEMO_HOME" setup
bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision 0 --actor demo-owner --reason 'public synthetic pilot only' \
  scope init --timezone UTC --synthetic
```

실제 Codex sandbox가 `/tmp` home을 읽지 못하는 환경에서는 승인된 private 경로 아래
새 fixture home을 만든다. sandbox를 완화하거나 기존 `.codex`/운영 디렉터리를
초기화하지 않는다. 기존 home이 있다면 `backup NEW_DIRECTORY` / `restore
BACKUP_DIRECTORY NEW_DATA_DIRECTORY`로 별도 복구하고, 복구 home의 큐가 paused임을
확인한 뒤 필요한 파일럿만 unpause한다. 백업은 자격증명을 포함하지 않는다.

`--synthetic` 기본값은 false다. 실제 자료를 가져온 뒤 true로 변경하는 경로는 없다.
실데이터 업무 기록은 이 옵션 없이 scope를 만들고 로컬 조회/변이만 사용한다.
scope/team/project는 다중 사용자 인증이나 원문 공개 권한이 아니다.

## 2. 업무와 일정

모든 변이는 최신 scope revision, actor, reason이 필요하다. 생성 receipt의 revision을
다음 명령에 사용한다. 충돌 시 `ops show`/`ops history`를 확인하고 의도된 새 명령을 만든다.

```sh
bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision 1 --actor demo-owner --reason 'confirmed owner commitment' \
  work create payment-check --title 'Verify payment success' --owner Mina \
  --completion-criteria 'Owner verifies successful payment' --due 2026-10-04T12:00:00Z

bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision 2 --actor demo-owner --reason 'provider investigation remains open' \
  work transition payment-check waiting --waiting-reason 'Provider investigating' \
  --responsible Mina --next-check 2026-10-04T13:00:00Z

bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision 3 --actor demo-owner --reason 'owner approved revised deadline' \
  work reschedule payment-check --due 2026-10-05T12:00:00Z
```

이 날짜는 고정 합성 사례다. 실제 업무에서는 승인된 RFC3339 offset 날짜를 입력한다.
waiting은 현재 due를 연장하지 않는다. reschedule은 최초 due와 변경 사유/시간/행위자를
보존한다. 완료에는 `work transition ID completed --evidence 'owner confirmed ...'`
처럼 확인 근거가 필요하다. 실제로 확인하지 않은 말을 넣어 통과시키지 않는다.

`--evidence`는 owner가 입력한 확인 문장이다. exact saved source/span 참조는
versioned `ops apply COMMAND_JSON`의 `data.evidence`를 사용한다. AI 보고가 내부
업무 상태를 자동으로 변경하지 않으며 외부 Jira Done도 완료 확인을 대신하지 않는다.

## 3. 릴리스 약속과 배포 근거

```sh
bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision 4 --actor demo-owner --reason 'confirmed staging cycle' \
  release create october-pilot --service checkout --environment staging \
  --ready-by 2026-10-05T12:00:00Z --deploy-by 2026-10-05T13:00:00Z

bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision 5 --actor demo-owner --reason 'include verified payment change' \
  release target october-pilot payment-check included
```

대상 제외는 `excluded`; 이월은 `carried --to-cycle LATER_ID`다. 같은 서비스/환경의
더 늦은 deployment deadline을 가진 cycle을 먼저 만들고 사유를 남긴다.
원래 약속/이월 이력은 사라지지 않는다.

실제 배포 증거가 있으면 `release deployment CYCLE deployed --environment ENV
--effective-at RFC3339 --work WORK_ID --evidence '...'`로 별도 저장한다.
`not_deployed`도 근거가 필요하다. 근거가 없는 상태는 `unknown`이며 미배포로
확정하지 않는다. 완료 업무/배포 안 됨, 배포됨/완료 미기록, 미완료/승인 이월을
`ops status --as-of RFC3339`에서 구분한다.

## 4. 저장 mail/Jira/Slack 입력

```sh
bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision 6 --actor demo-owner --reason 'saved public Slack fixture' \
  observation import slack-capture examples/team-ops/saved-slack.json --format slack
```

mail 형식은 기존 saved Snapshot, Jira 형식은 기존 ReportInput이다. 각각
`examples/mail/synthetic-day.json`, `examples/jira/report-input.json`을 새 revision으로
import할 수 있다. 이 fixture들의 외부 업무가 현재 ledger와 실제로 연결되어 있다고
가정하지 않는다. AI는 대응 후보/매핑 불확실성을 표시해야 한다.

`ops sources OBSERVATION_ID`는 정확한 source/version/span ID와 수집 구간·공백을
보여준다. 원본 bytes와 digest는 event history에 보존된다. 중복 원본과 같은 version의
내용 충돌은 거부한다. 새로운 내용은 새 version/capture로 입력한다. 수정/삭제와
늦게 도착한 근거를 숨기지 않는다. 불완전 수집은 gaps를 명시한다.
저장 범위의 complete는 연속 live monitoring이 아니라 저장 capture의 범위 선언이다.

## 5. 실제 AI 보고

설치된 Codex CLI의 exact tool-free structured 경계와 승인된 모델 route가 필요하다.
API/key/provider를 새로 만들거나 설치 버전이 다르다는 이유로 sandbox를 완화하지 않는다.
현재 파일럿에서 검증한 installed CLI/version/route는 검증 기록에 있다.
운영 home의 설정 변경은 별도 선택이다. 필요한 role 설정은 기존 `config route`,
`config policy`, `doctor` 안내를 사용하고 직접 private source 공개 승인을 우회하지 않는다.

먼저 기존 baseline Skill을 정확한 project namespace에 초기화한다. 추가 Skill은
[Scoped Skills 안내](SCOPED_SKILLS.md)의 명시적 조합/검토를 따른다.

```sh
bin/chunsu --home "$OPS_DEMO_HOME" skills initialize --workgroup team-ops \
  --skill-scope project --team pilot --project checkout \
  --actor demo-owner --reason 'select existing baseline for synthetic pilot'

bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout report \
  --objective 'Compare work deadlines, release commitments, deployment coverage and current Slack impact; separate historical resolved urgency.' \
  --as-of 2026-10-06T00:00:00Z
bin/chunsu --home "$OPS_DEMO_HOME" --json flow run JOB_ID
bin/chunsu --home "$OPS_DEMO_HOME" --json flow inspect JOB_ID
bin/chunsu --home "$OPS_DEMO_HOME" --json flow result RESULT_EVENT_ID
```

collect/refine는 실제 host 단계이고 synthesis는 실제 모델이다. 지원하지 않는 경계는
unavailable로 멈춘다. 보고의 partial은 수집/최신성/모델 판단 한계가 있다는 뜻이다.
result delivery가 available이라는 표시도 외부 메신저에 sent라는 뜻이 아니다.

같은 입력/Skill version을 비교하려면 ledger를 변경하지 않고 동일 objective와
`--as-of`로 baseline `ops report`와 `ops report --skill-candidate DIGEST`를 각각
실행한다. legacy `experiment`는 team-ops FLOW ancestry를 보존하지 않으므로 사용하지
않는다. [FLOW feedback 안내](FLOW_FEEDBACK.md)에 따라 실제 실행 근거를 기존
feedback/독립 review에 연결한다. 후보는 비활성이며 사람이 검증/비교 후 채택한다.
자동 adoption은 없다.

## 6. 보고를 shadow 대응에 반영

현재 scope revision을 확인하고 verified JOB만 적용한다.

```sh
bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout \
  --revision CURRENT_REVISION --actor demo-owner --reason 'review verified saved report' \
  --operation-id OWNER_CHOSEN_STABLE_ID reconcile JOB_ID
bin/chunsu --home "$OPS_DEMO_HOME" --json ops --team pilot --project checkout outbox list
```

reconcile는 실제 실행 근거/input/Skill/output과 현재 ledger를 재확인한다.
ledger가 보고 후 변경됐으면 stale이라 거부하며 fresh 보고를 요청한다.
업무/일정/배포는 변하지 않는다. 정확한 operation ID의 동일 재시도는 처음 receipt만
돌려주며 incident/알림 계획을 다시 만들지 않는다.

명시적 shadow 정책 예시 `examples/team-ops/shadow-policy.json`을 `policy set FILE`
로 저장할 수 있다. demo recipient는 실제 계정이 아니고 sender도 없다. 기본 recipients는
비어 있어 `shadow_blocked`다. cooldown, 응답 기한, freshness, P0 snooze 우회는
owner가 정책에 넣은 값만 사용한다. P3 none은 로컬 현황에만 남긴다.

incident ID는 `ops status`에서 조회한다. `incident ack/respond/resolve/reopen/snooze`
는 최신 revision/actor/reason이 필요하다. ack는 해결이 아니며 resolve는 evidence를
요구한다. snooze는 미래 `--until RFC3339`를 가진다. 억제 만료/정책 변경 후
`outbox review`를 실행하면 현재 shadow preview를 append한다. 같은 현재
policy/status/reason으로 반복 review해도 중복 계획은 생기지 않는다.
이 명령은 실제 송신 재시도, 자동 follow-up 또는 미확인 상향 호출이 아니다.

## 7. 불명확한 결과와 복구

mutation의 응답을 잃었으면 error에 나타난 operation ID 또는 미리 선택한 ID로
`ops receipt ID`/`ops history`를 확인한다. 정확히 같은 명령/예상 revision/ID만
재시도한다. 다른 내용으로 ID를 재사용하거나 최신 revision을 자동 끼워 넣지 않는다.

history 변조/누락/byte budget 초과는 전체 조회/변이를 차단한다. 일부 최신 record만
읽어서 정상처럼 보이지 않는다. current config 한도를 낮춰 차단했다면 승인된
설정/백업을 복구하고 원인을 확인한다. ledger compaction은 아직 없다.
writer lock이 있으면 owner CLI는 controller management channel을 쓰고, 별도
`flow run` writer는 거부한다. 운영 DB를 두 프로세스의 독립 writer로 열지 않는다.

실제 사내 도입은 저장/공개/보존 계약, 사용자 권한, 당번/수신자, live collector,
provider 송신·불명 처리, background follow-up, 단말/VPN/SSO/외부 heartbeat와
백업 복구를 별도 검증한 뒤 진행한다.
