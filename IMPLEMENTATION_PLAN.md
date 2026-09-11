# Chun-su Phased Implementation Plan

**Version:** 2.0  
**Date:** 2026-09-05  
**Status:** Implementation through Phase 5 and the approved bounded Jira/Skill integration implemented; human acceptance and background activation remain pending
**Confirmed stack:** Go + embedded SQLite  
**Current position:** MOD-01–06 implement optional selected-result review and release requirements, separate reception and execution routes, portable Linux operation, bounded Git review and owner access/revocation. Final installation checks are recorded in MOD-07; human usefulness and company deployment remain separate evidence.

On 2026-09-11 the user authorized the [modular agent implementation goal](docs/implementation/MODULAR_AGENT_GOAL_2026-09-11.md):
human-owned criteria for execution/review/analysis/optimization, on-demand review
of accumulated results, separate reception and task agents, replaceable execution
environments, and lightweight local/Linux/server operation around a single SQLite
controller. That goal records the latest authorized direction and bounded work
units. Earlier deferrals of those implementation areas are superseded within its
scope; actual company access, personal acceptance and external deployment still
need their own evidence.

The [2026-09-10 evaluation and release-gate review](docs/implementation/STATUS_2026-09-10.md)
records the latest code inspection, focused existing checks, and proposed next
Phase 3 work. Comparison eligibility is not a quality release gate; evaluator
execution isolation and adoption criteria remain explicit preparation items.
This checkpoint does not authorize new policy, interface or permission changes.

On 2026-09-10 the user also requested online research and bundled setup manuals
for Drive (Docs/Sheets), Figma, GitHub, Slack, Telegram and Gmail, with AI-assisted
or fixed-script installation paths. The [integration onboarding preparation](docs/implementation/INTEGRATION_ONBOARDING.md)
and [setup entrypoint](docs/setup/INTEGRATIONS.md) record the prepared assets and
bounded implementation units. Preparation does not establish six live connectors
or authorize account access, external writes, or a new executor permission model.

On 2026-09-11 the user approved implementing and running the setup conversation
and its processing host after the failed entrypoint walkthrough. The
[setup conversation checkpoint](docs/validation/SETUP_CHAT_2026-09-11.md) records
the bounded delivery: guided local setup, six-service manual installation,
asynchronous Gmail connection/verification through the host, and preserved report
executor permissions. Other live adapters and public distribution remain pending.

The user then requested natural chat with executable actions restricted by the host.
The [AI chat checkpoint](docs/validation/AI_CHAT_2026-09-11.md) records the configured
Codex conversation path, explicit Skill, typed capabilities and actual dialogue
checks. The earlier fixed setup flow remains under `chat --guided`. This does not
complete other live adapters, report admission from chat, or the quality release gate.

The user subsequently selected Telegram for a basic chat test. The bounded
[IN-05 private chat implementation](docs/implementation/TELEGRAM_CHAT.md) adds
local token entry, owner pairing and private DM replies using the existing AI/action
boundary. [Validation](docs/validation/TELEGRAM_CHAT_2026-09-11.md) separates the
mock Bot API plus real Codex checks from the still-pending live bot pairing.

## 1. Purpose and scope

Build a small personal framework that prepares work for a replaceable AI
executor, enforces human-owned controls outside that executor, preserves
evidence, and helps the user improve recurring work.

The first useful milestone is a saved-input mail report with a complete
result-evaluation, improvement, and revalidation loop. A persistent service is
not a prerequisite for that milestone.

On 2026-09-06 the user authorized implementation through Phase 5 and selected
Gmail for live validation, with connection requests made when needed. After the
bounded mail pilot, the user expanded scope to Jira collection preparation with
a replaceable provider-to-internal-data boundary, deferring the real connection.
The [Jira ingestion proposal](docs/implementation/JIRA_INGESTION_PREPARATION.md)
defines the approved collection units and remaining provider decisions. The
[saved ingestion checkpoint](docs/validation/PHASE_06_JIRA_INGESTION.md) records
implemented readers, normalization and preservation; live HTTP access and
Jira report execution remain deferred.
This does not authorize tenant access or complete the full Phase 6 report loop.
Task acceptance scenarios are planned checks until evidence is recorded.

On 2026-09-07 the user subsequently authorized read-only Jira access and a
bounded grouped manual report, then requested a mail/Jira execution-gate and
independent golden-evaluation loop. The [Jira integration proposal](docs/implementation/JIRA_AUTOMATION_IMPLEMENTATION.md)
records the selected source scope and remaining exact integration contracts.
The [execution/evaluation checkpoint](docs/validation/EXECUTION_GOLDEN_LOOP_2026-09-07.md)
records actual attempts and checks. The manual Jira report does not establish
common-framework execution, and neither synthetic success nor AI evaluation
establishes human acceptance or authorization to enable schedules.

On 2026-09-08, a fresh authorized Jira GET capture was ingested as three
non-synthetic saved-reader acquisitions. The [real-source checkpoint](docs/validation/JIRA_REAL_SOURCE_INGESTION_2026-09-08.md)
records raw-byte preservation and remaining v1/report limitations. This does
not implement the live connector or complete Jira executor/gateway evaluation.

The user subsequently accepted the Jira integration proposal and required
workgroup instructions to be managed as Skills and explicitly injected into
each executor attempt. The proposal records this approval and the delivery
requirements. Jira integration and Skill delivery are authorized work, not
completed functionality; their prior contract-confirmation blocker is resolved.

The subsequent implementation is recorded in the
[Jira/Skill integration checkpoint](docs/validation/JIRA_SKILL_INTEGRATION_2026-09-08.md).
It supersedes the implementation deferrals above for the approved grouped
scope: a live Cloud connector, explicit per-attempt Skills, the common Jira
execution/evaluation path, and disabled interval scheduling are implemented.
Real Codex runs produced synthetic Jira, live Jira and mail regression reports.
This does not establish capacity/sprint reporting, human usefulness, candidate
adoption, or authorization to activate a recurring schedule.

Go and SQLite are decided. Do not reopen that comparison without new evidence.
The executor, provider-specific permissions, personal mail semantics, and other
open decisions remain explicit dependencies of the tasks that need them.

## 2. Document authority

Current user decisions take precedence over this plan. Preserve the following
principles when refining implementation details:

1. Humans own active rules, skills, tool permissions, schemas, completion
   policies, evaluation criteria, and promotion decisions.
2. Executors choose how to perform work within the declared scope. The
   controller must not turn each reasoning step into a human approval.
3. Permission enforcement, minimum output validity, operational completion,
   and semantic quality evaluation are different responsibilities.
4. Source evidence, failures, retries, uncertainty, and historical results
   remain traceable. An executor's success declaration is not completion proof.
5. Result evaluation and optimization are independent axes. Neither is
   obligatorily run after every job, and optimization does not require growing
   the golden set.
6. AI-generated changes remain proposals until a human adopts them.
7. Mail reporting comes first, Jira reporting second, and development work
   follows validation of the common framework.
8. Initial mail and Jira workflows are read-only. Mail read-state changes,
   replies, archive/delete/move/label actions, calendar changes, and Jira
   comments/transitions/field updates are outside that scope.

| Document | Role now |
|---|---|
| [Working principles](WORKING_PRINCIPLES_AND_DIRECTION_2026-09-05.md) | Primary product principles and mail/Jira requirements; preserved source |
| This plan and its phase files | Current implementation sequence, work units, dependencies, and completion evidence |
| [Readiness review](IMPLEMENTATION_READINESS_REVIEW.md) | Unresolved semantics, requirement traceability, and evaluation risks |
| [Local runtime design](LOCAL_RUNTIME_STACK.md) | Supporting implementation recommendations; Go + SQLite is now selected |
| [Detailed workflow design](WORKFLOW_DETAIL_DESIGN.md) | Reference for packaging, gateway, credentials, and evidence; older Jira mutation examples are not current scope |
| [Direction report](DIRECTION_REPORT.md) and [chat brief](CHAT_DISCUSSION_BRIEF.md) | Historical reasoning and discussion material |
| [Original design](DESIGN.md) | Historical architecture reference; not proof of existing code |
| [Superseded implementation plan](docs/archive/IMPLEMENTATION_PLAN_2026-09-05_SUPERSEDED.md) | Preserved verbatim; do not execute its kernel-first or managed-provider roadmap |

The project began as a documentation-only directory. At the user's request,
Git was initialized in the current checkout and the pre-implementation inputs
were preserved in commit `7020143`. Implementation now proceeds under that authorization. No
remote has been configured or pushed. Recheck the working tree before each
change and follow the [repository instructions](AGENTS.md).

## 3. Implementation baseline

The following working defaults implement the user's lightweight, terminal-first
direction. Phase 0 records concrete versions and contracts before they are
encoded. They are not additional product promises.

| Area | Working baseline |
|---|---|
| Core | Go; standard library first; Cobra is the proposed CLI library |
| Persistence | Embedded SQLite through a reviewed driver; `modernc.org/sqlite` is the current candidate |
| Database use | Explicit SQL, local disk, short transactions, one controller writer; WAL and durability settings recorded explicitly |
| Work coordination | One active AI job initially; durable job/attempt records; in-memory signals only wake workers |
| User assets | Markdown instructions and examples; JSON configuration and versioned result schemas |
| Initial use | Manual CLI runs on the current macOS/arm64 host |
| Initial report delivery | Terminal output and a local Markdown artifact; no external message channel |
| Executor integration | One adapter, selected in Phase 0; API, CLI, or SDK details remain adapter concerns |
| Service operation | Optional macOS user LaunchAgent, added after useful manual operation |
| Secrets | Host-managed secret references; macOS Keychain integration selected and verified before live credentials |
| Expansion | OS-specific service, secret, path, process, and isolation behavior behind platform interfaces |

A Go release binary should not require users to install the Go compiler or a
SQLite CLI. The chosen AI executor and optional extensions may still require
their own runtimes, accounts, or sandbox setup.

Do not introduce Redis, PostgreSQL, a separate queue broker, a web dashboard,
a distributed scheduler, multiple executors, or a mandatory cloud provider
into the first milestone. The isolation requirement still applies: a normal
same-user child process is not a sandbox. Select a supported executor boundary;
introduce an additional isolation mechanism only if necessary to enforce it.

### 3.1 Responsibility boundaries

```text
Human-owned workgroup assets and connection bindings
             |
             v
Go controller: pin versions, prepare request, own lifecycle
             |                         |
             v                         v
Executor package                 Gateway policy snapshot
             |                         |
             v                         v
Replaceable AI executor ---> scoped tools ---> host connectors
             |                                  |
             +------------ outputs / evidence --+
                                      |
                                      v
                 validation, completion, local report
                                      |
                  +-------------------+------------------+
                  v                                      v
          result evaluation                     optimization review
                  +-------------------+------------------+
                                      v
                      proposal -> validation -> human adoption
```

The gateway and controller are modules in one program initially. Service
credentials stay in host connectors. Executor-visible instructions and
machine-enforced policy derive from the same pinned declarations. Golden
answers, active-control write access, management channels, and unrelated
history are excluded from the executor's authority.

### 3.2 Storage and contract boundaries

Phase 0 specifies ownership and meaning; exact field names and SQL tables are
reviewed before implementation.

| Record or asset | Meaning and owner |
|---|---|
| Workgroup definition | Human-owned task instructions, capabilities, public completion requirements, and schema references |
| Request and input snapshot | Controller-owned scope, as-of time, timezone, target sources, reference-only history, and acquisition status |
| Execution package | Pinned copies of permitted assets, initial input, tool descriptions, and output instructions |
| Job and attempt | Controller-owned lifecycle and execution identity; retries remain separate attempts |
| Evidence and artifacts | Actual input/tool responses/errors and collected outputs; fixed after collection |
| Validation and completion | Host checks and policy decisions, independent of evaluator scores |
| Evaluation | Versioned expectations, judgments, uncertainty, and evidence linked to a result |
| Optimization review | Findings across one or more runs and possible changes to work rules, tools, schemas, and procedures |
| Proposal and adoption | Candidate changes, validation results, and human acceptance or rejection |
| Connection and secret reference | Non-secret configuration separated from credential values |

SQLite stores state and relationships. Files store workgroup assets and larger
snapshots, responses, and reports. DB schema version, output schema version,
tool contract version, evaluator version, and asset version are distinct.

Acquisition progress, source disposition, report creation, and delivery
acknowledgment are separate facts. A retrieval cursor must not cause
unprocessed material to disappear after a crash.

## 4. Roadmap and phase files

The ten phase files contain 70 bounded tasks with IDs, dependencies,
deliverables, and observable acceptance evidence. Numbering is a navigation
order, not a demand to implement every phase before obtaining value.

| Phase | Outcome | Main dependency | Current state |
|---|---|---|---|
| [0 — Decisions and mail contracts](docs/implementation/PHASE_00_DECISIONS.md) | Startable foundation work and explicit blockers for the first mail run | Current principles and Go + SQLite choice | Core contracts prepared; executor/personal policy decisions pending |
| [1 — Minimal CLI and durable records](docs/implementation/PHASE_01_FOUNDATION.md) | A small runnable program with persistent job/artifact records | P0-08 | Foundation checked; real process lifecycle evidence pending |
| [2 — Saved-input mail execution](docs/implementation/PHASE_02_SAVED_MAIL.md) | A real executor produces a traceable local mail report | Phase 1 and first-run decisions | Real nine-source synthetic report and scoped access verified; broader lifecycle evidence pending |
| [3 — Evaluation and feedback](docs/implementation/PHASE_03_FEEDBACK.md) | One complete evaluation/change/revalidation/adoption-or-rejection cycle | Phase 2 | Feedback tooling checked; MS1 remains pending |
| [4 — Live read-only mail](docs/implementation/PHASE_04_LIVE_MAIL.md) | Useful reports from one explicitly scoped account | Phase 3 and live-data decisions | One 20-message batch collected and candidate report produced; partial scope and human review remain explicit |
| [5 — Repeated local operation](docs/implementation/PHASE_05_OPERATIONS.md) | Queue recovery, schedules, terminal intervention, backup, and optional service | Manual live-mail pilot | Operations commands and synthetic checkpoints prepared; live MS3 pending |
| [6 — Jira report reuse](docs/implementation/PHASE_06_JIRA.md) | The same core supports a second workgroup | Phase 3 and manual mail pilot; service not required | Approved grouped live report, explicit Skills and independent evaluation verified; wider phase and human acceptance remain pending |
| [7 — Installation and distribution](docs/implementation/PHASE_07_DISTRIBUTION.md) | A reviewable release package for a declared support scope | Useful operation and selected release scope | Bounded setup dialogue implemented; release scope and packaging pending |
| [8 — Deeper optimization](docs/implementation/PHASE_08_OPTIMIZATION.md) | Evidence-based improvements across runs without mandatory golden-set growth | Phase 3 and sufficient recorded use | Not started |
| [9 — Development-work pilot](docs/implementation/PHASE_09_DEVELOPMENT.md) | One bounded repository task with appropriate controls and evidence | Mail/Jira reuse evidence and an explicit development scope | Not started; conditional |

The critical path to the first value demonstration is **0 -> 1 -> 2 -> 3**.
Phase 4 proves live usefulness. Phases 5 and 6 can proceed as separate tracks
after that manual pilot. Phase 8 does not have to wait for public distribution,
and Phase 9 does not require every optional optimization capability.

Milestone claims are deliberately separate:

- **MS0 — Ready to scaffold:** foundation decisions recorded; later blockers named.
- **MS1 — First feedback loop:** saved-input mail execution and useful feedback
  complete. Synthetic evidence is labeled as such.
- **MS2 — Live mail usefulness:** real authorized inputs and user review support
  the claim; absence of external mutations is checked.
- **MS3 — Repeatable local operation:** interruption, resume, retention, and
  delivery behavior are demonstrated.
- **MS4 — Reuse:** Jira fits the same lifecycle/evidence/evaluation system.
- **MS5 — Distributable package:** installation and operation verified for the
  explicitly declared OS/CPU and executor combination.

## 5. Decision register

Do not ask again for Go + SQLite, mail-first scope, or the two independent
improvement axes. Prepare examples and a recommendation for each remaining
decision; ask only when that task actually depends on the answer.

| ID | Decision still needed | Prepare in | Must be resolved before |
|---|---|---|---|
| D01 | First executor, allowed model environment, invocation method, authentication, budget, and enforceable boundary | P0-05 | Actual model execution in P2-03/P2-04 |
| D02 | Mail categories/overlap, user role, importance/spam rules, history meaning, delta versus outstanding actions, and partial-report policy | P0-03/P0-04 | Personal semantic contracts and claims in P2-01/P3-01 |
| D03 | Which saved data may be sent to the chosen executor; public examples versus private expectations | P0-04/P0-05 | First model execution |
| D04 | Minimal request/result/evidence/state contracts and versioning; later contracts remain incremental | P0-06/P0-07 | Corresponding schema and external interface code |
| D05 | Mail provider, account, source scope, attachments, and related-service reads | P4-01 | Live connector integration |
| D06 | Live-data disclosure, retention/deletion, secret-store behavior, and allowed diagnostic content | P4-01/P4-02 | Credential handling or persistence/transmission of live data |
| D07 | Scheduling, missed-run handling, waiting-for-user behavior, and optional external delivery | P5-01 | Unattended operation or adding a delivery destination |
| D08 | Jira deployment type, identity, sprint definition, risk rules, estimates and real available capacity | P6-01 | Jira-specific contracts and live access |
| D09 | Supported OS/CPU/features, release channel, signing and update approach | P7-01 | Release support claims or publication |
| D10 | Baseline quality, unacceptable errors, repeat sample needs, and adoption criteria | P3-01/P3-07 | Claims that quality or user effort improved |
| D11 | Whether/when recurring optimization is useful, review budget, and candidate selection | P8-01 | Recurring optimization operation |
| D12 | Development task type, repository, allowed changes, existing validation, and external-effect authority | P9-01 | Any target-repository development run |

Unknown provider details must not block local configuration or SQLite work.
Unknown personal semantics may remain explicit in a synthetic demonstration,
but cannot be silently replaced with invented user preferences.

For the approved saved Jira ingestion subset, D04 is resolved by the versioned
[evidence contract](docs/contracts/jira-evidence.md). D08 remains open for the
real deployment, authenticated identity, field discovery and report judgments;
the saved policy requires explicit fictional scope in public examples.

On 2026-09-06 the user authorized the existing AI account and one scoped live
trial after access verification. D01/D03/D05/D06 are resolved for that trial:
the tested Codex CLI/model, the connected Gmail account's seven-day inbox policy,
at most 20 listed messages, no thread expansion, local reports and manual
retention. The [executor verification](docs/validation/EXECUTOR_BOUNDARY_2026-09-06.md)
records the actual supported boundary and limitations. D02 is represented by
the [candidate reporting baseline](docs/contracts/mail-pilot-reporting.md),
without silent active-rule adoption. This authorized early live experiment does
not complete the Phase 3 human feedback loop or close MS1/MS2.

## 6. Work-unit and validation rules

- Execute one bounded task or a coherent set of dependent tasks at a time.
  Keep each change reviewable; split a task when it introduces another
  unresolved contract or independently deliverable behavior.
- Task states are `not_started`, `ready`, `in_progress`, `blocked`, `deferred`,
  and `complete`. Completed work includes evidence; blocked work names the
  exact missing decision and unaffected work that can continue. Deferred work
  is deliberately outside the selected scope, such as an unselected OS target.
- A phase boundary is not itself an approval gate. Continue authorized,
  reversible work. Before encoding an unresolved high-risk architecture,
  schema, permission, or external-interface choice, prepare the concrete
  proposal and obtain the required user decision.
- Work in the current checkout. Do not create a worktree or invent a remote.
- Commit completed, validated work as it becomes ready. Use a bounded task or
  coherent group of tasks as a commit boundary; a large phase may contain
  several commits. Include the phase/task ID and concrete outcome in the
  commit subject, and record relevant validation and remaining limits in the
  body when helpful. Example: `feat(phase-1): P1-03 initialize durable SQLite state`.
- Stage only the files for that work unit, inspect the staged changes, and
  exclude credentials and runtime work data. Update task status with its
  evidence. Do not label a phase complete merely because one task is committed.
  Local commits are authorized; remote creation, push, and publication are
  separate actions and are not part of this planning request.
- Do not create test code unless the user explicitly requests it. Run relevant
  existing tests when available. The scenarios in these phase files specify
  acceptance evidence, not authorization to generate a new test suite.
- Product evaluation cases are versioned inputs and human-reviewed expectations,
  not fabricated evidence of real-world quality. Synthetic cases remain labeled.
- Validate changed behavior and directly affected integration points in a
  coherent batch. Avoid broad validation without a cross-cutting reason.
- Build/format/static checks, manual scenario execution, and existing tests
  have different purposes. Record what actually ran, what did not, and why.
- After the same approach fails twice for the same cause, stop that approach,
  preserve evidence, and report the needed decision instead of widening scope.
- Do not hard-code credentials, endpoints, paths, personal rules, or unnamed
  operational limits. Use reviewed configuration and named defaults.
- Do not collect private chain-of-thought. Store observable actions, source
  evidence, result rationale, errors, and measured resource information.
- Public releases, service activation, real account access, and external
  messages require authority appropriate to the concrete action. Use the latest user authorization; request only the missing account/scope
  decisions when a concrete integration is ready.

A task's completion note should contain: task ID, changed behavior, artifact
paths, checks actually run and their results, checks not run, remaining
limitations, and the next dependency made ready. Keep evidence local and
exclude secrets. No numeric quality threshold is invented in this plan.

## 7. Repository and runtime layout

Only the planning documents exist as a result of this planning task. The
following application structure is a target, to be created incrementally:

```text
cmd/chunsu/                  CLI entry point
internal/cli/                commands and terminal presentation
internal/app/                preparation, lifecycle, queue and completion
internal/store/              SQLite, migrations and artifact references
internal/executor/           first adapter and its capability boundary
internal/gateway/            scoped tools and observable call evidence
internal/connectors/         mail, then Jira
internal/evaluation/         judgments, comparisons and proposal links
internal/platform/           paths, secrets, services and process behavior
workgroups/                  versioned guides, public examples and schemas
examples/synthetic/          non-production inputs clearly labeled
docs/decisions/              reviewed choices and unresolved decisions
docs/contracts/              meaning, ownership and versioned interfaces
docs/operations/             setup, recovery, backup and support guidance
docs/validation/             actual task/milestone evidence
```

Runtime data is derived from OS user paths, with a documented override such as
`CHUNSU_HOME`. It contains config, SQLite state, snapshots, reports, logs,
evaluation records, and proposals. Real mail, tokens, and runtime databases
must not be mixed into the source tree or published example assets.

## 8. Starting sequence

1. Read [Phase 0](docs/implementation/PHASE_00_DECISIONS.md). P0-01 records the
   reconciliation already done by this planning change.
2. Start P0-02 for dependency/build choices and P0-03 for concrete mail examples.
   These are independent investigations; no delegation is required.
3. Prepare the first executor capability assessment and semantic contract
   drafts. Leave unresolved personal/provider choices visible.
4. Complete P0-08 for the foundation portion of the decision set, then implement
   Phase 1. Continue resolving first-mail-run decisions without delaying work
   that does not depend on them.
5. Finish Phases 2 and 3 before expanding infrastructure for its own sake.

This plan is complete when its task references and document links are valid,
the superseded plan is preserved, and Go + SQLite and the revised sequence
are consistent across the current entry documents. It is not evidence that
any implementation milestone has been achieved.

The completed documentation checks and remaining Phase 0 work are recorded in
the [P0-01 planning completion note](docs/validation/PHASE_00_PLANNING.md).

Implementation checkpoints are recorded in `docs/validation/PHASE_01_FOUNDATION.md`, `PHASE_02_SAVED_MAIL.md`, `PHASE_03_FEEDBACK.md`, `PHASE_04_GMAIL.md`, and `PHASE_06_JIRA_INGESTION.md`. Code preparation does not establish useful-feedback, live-usefulness, repeated-operation or Jira-report reuse milestones. The approved Jira ingestion subset is complete for saved inputs; live Jira access, Jira AI reporting and later phases remain outside this execution scope.
