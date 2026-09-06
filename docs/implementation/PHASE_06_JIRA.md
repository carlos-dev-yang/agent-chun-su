# Phase 6 — Jira Reporting as a Reuse Check

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** Collection/normalization/preservation scope approved; implementation in progress; real connection deferred

**Goal:** Prove that a second workgroup fits the existing execution, evidence, evaluation, and proposal structure.

**Entry conditions:** P3-07 and P4-09 are complete. Jira can start as a manual workflow; Phase 5 service completion is not required.

**Exit evidence:** One scoped Jira report supports the user's assigned/current-sprint decisions and is evaluated through the same common framework.

**Out of scope:** Ticket implementation, comments, field changes, transitions, reassignment, and development execution.

The latest user request authorizes Jira collection preparation now, while
deferring the real account connection. This is an explicit early subset of
Phase 6, not acceptance of the full phase's entry/exit milestones. The
[ingestion preparation proposal](JIRA_INGESTION_PREPARATION.md) specifies the
replaceable reader/normalizer boundary, reuse points, concrete work units and
completion evidence. The user approved this collection and internal-data
boundary. Cloud/Data Center selection remains open; tenant access and AI Jira
reporting are deferred.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P6-01 — Define Jira reporting semantics and access scope

- **Status:** in_progress
- **Depends on:** P3-07, P4-09; D08.
- **Work:** Select Cloud or Data Center integration as appropriate, user identity, project/board/sprint scope, necessary history, and allowed reads. Define assigned work versus sprint work, delay versus risk, meaningful post-completion issues, and available-capacity evidence.
- **Deliverable:** A Jira workgroup brief and concrete connection/semantic decision.
- **Acceptance evidence:** The report can distinguish confirmed facts, risk, and missing information. Ticket counts are not substituted for effort, and absent capacity data is not invented.

### P6-02 — Implement one scoped Jira read connector

- **Status:** not_started
- **Depends on:** P6-01; applicable D06 data policy.
- **Work:** Verify current official APIs and implement the chosen authentication, identity, paginated issue/history reads, relevant relationships, and rate/error handling. Reuse host secret references and gateway enforcement.
- **Deliverable:** One Jira connector and bounded tool bindings.
- **Acceptance evidence:** Assigned and sprint sources are obtained within the approved scope; other people's work is not counted as the user's workload. Needed dependency context is still constrained by allowed access. No write endpoint is exposed.

### P6-03 — Normalize Jira evidence and effective time

- **Status:** not_started
- **Depends on:** P6-02.
- **Work:** Preserve issue/history identity, as-of time, status changes, estimates, dependencies, and evidence of follow-up after completion. Handle unavailable history and partial pages explicitly.
- **Deliverable:** A Jira input/evidence adapter using the common envelope.
- **Acceptance evidence:** An ordinary comment after completion is not automatically a new action. Late/partial evidence cannot be presented as a complete historical snapshot.

### P6-04 — Create the Jira guide and result contract

- **Status:** not_started
- **Depends on:** P6-01, P6-03; review the new workgroup contract.
- **Work:** Define report sections for assigned work, current sprint, confirmed delays/risks, post-completion follow-ups, and capacity/feasibility with stated assumptions. Keep Jira meanings in the workgroup/connector, not the generic controller.
- **Deliverable:** `workgroups/jira-report/` with guide, schema, public examples, and scoped inputs.
- **Acceptance evidence:** The same lifecycle and artifact/evaluation envelopes accept Jira without a parallel framework. The report expresses unavailable estimates/capacity as unknown rather than false precision.

### P6-05 — Implement evidence-based capacity presentation

- **Status:** not_started
- **Depends on:** P6-04.
- **Work:** Apply only the agreed resource calculation and units, using estimates and real availability supplied through an authorized source or explicit user input. Separate blockers, assumptions, and uncertainty; do not add calendar access implicitly.
- **Deliverable:** Capacity input validation and report presentation within the Jira workgroup.
- **Acceptance evidence:** Missing estimates or availability produce a useful bounded conclusion. A change in the calculation is versioned and does not rewrite previous reports.

### P6-06 — Evaluate saved and live Jira reports

- **Status:** not_started
- **Depends on:** P6-05, P3-02.
- **Work:** Prepare reviewed cases for sprint changes, blocking dependencies, incomplete history, post-completion updates, and missing capacity. Run saved cases, then the approved live scope, through existing evaluation and feedback paths.
- **Deliverable:** Jira report/evaluation bundles and an observed-issue review.
- **Acceptance evidence:** Semantic judgments are linked to source evidence, and failed/unknown outcomes remain visible. Read-only reporting is not claimed to complete the underlying Jira work.

### P6-07 — Review reuse and close MS4

- **Status:** not_started
- **Depends on:** P6-06.
- **Work:** Inspect which code and contracts changed for the second workgroup. Identify mail-specific assumptions that leaked into the core and propose only necessary corrections with both workflows in the validation scope.
- **Deliverable:** The MS4 reuse note and focused follow-up tasks if needed.
- **Acceptance evidence:** Jira shares request/attempt/evidence/evaluation/proposal mechanics. Mail still works after any core correction. Executor interchangeability remains an architectural intent unless a second adapter has actually been checked.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.
