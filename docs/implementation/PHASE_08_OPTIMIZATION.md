# Phase 8 — Deeper Optimization Across Recorded Runs

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** MOD-01 selected-result analyzer/optimizer execution and synthetic candidate comparison implemented; human usefulness/adoption remains pending.
**Goal:** Improve the operating rules, tools, schemas, skills, and process using accumulated evidence while preserving independent result evaluation.

**Entry conditions:** P3-07 has established the independent record/proposal model, and enough representative runs exist to investigate a concrete issue. Jira and public distribution are not prerequisites.

**Exit evidence:** At least one evidence-backed optimization hypothesis has been investigated and compared, with a documented human decision. No mandatory golden-set expansion is required.

**Out of scope:** Automatic active-rule promotion, private chain-of-thought collection, optimization after every job, and a single combined quality/process score.

The approved [modular-agent goal](MODULAR_AGENT_GOAL_2026-09-11.md) adds versioned
review criteria and optional review over explicitly selected saved results.
[MOD-01 evidence](../validation/MOD_01_SELECTED_REVIEW_2026-09-11.md) records the
actual synthetic investigation/comparison and negative adoption checks. The
original work units below still need a human-selected representative issue and
decision before claiming this phase's usefulness milestone.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P8-01 — Define a bounded optimization review

- **Status:** not_started
- **Depends on:** P3-07; representative run evidence; D11.
- **Work:** Choose a question, run cohort, review budget, and on-demand or recurring mode. Use actual run volume and user burden instead of an invented fixed schedule.
- **Deliverable:** An optimization review definition and selected evidence references.
- **Acceptance evidence:** The review has a concrete question and stopping condition. Selecting a recurring mode does not itself create an automation or authorize repeated spending.

### P8-02 — Build an evidence view across runs

- **Status:** not_started
- **Depends on:** P8-01.
- **Work:** Join observable inputs, tool outcomes, errors, result judgments, versions, duration/cost where available, and user corrections. Surface truncated/missing data and separate observations from interpretations.
- **Deliverable:** A cross-run evidence report or terminal inspection path.
- **Acceptance evidence:** The view includes successful, failed, and unknown outcomes. It does not infer hidden reasoning or classify an alternative valid tool sequence as wrong by default.

### P8-03 — Create a targeted workflow improvement candidate

- **Status:** not_started
- **Depends on:** P8-02, P3-04.
- **Work:** Select a supported hypothesis about a guide, skill, tool response, schema, configuration, or work process. Keep the changed surface narrow and state the expected outcome and any permissions/interface impact.
- **Deliverable:** A candidate and its validation scope using existing proposal records.
- **Acceptance evidence:** The proposed cause is labeled with its evidence strength. The candidate can be valid without adding or modifying any golden case.

### P8-04 — Validate quality and the changed mechanism

- **Status:** not_started
- **Depends on:** P8-03, P3-05.
- **Work:** Use relevant existing cases for output quality and direct checks of the changed tool/schema/permission mechanism. Compare user burden and resource use where measured; qualify independent-validation limits.
- **Deliverable:** A candidate comparison and mechanism-specific evidence.
- **Acceptance evidence:** A better golden score alone does not establish tool correctness, permission safety, or reduced user effort. Both improvements and regressions are visible.

### P8-05 — Record adoption and decide future review cadence

- **Status:** not_started
- **Depends on:** P8-04, P3-06; D11 for recurring operation.
- **Work:** Obtain the human adoption/rejection decision and preserve active-version history. Decide whether further reviews are worth their cost. Enable recurring review only within an explicitly selected operating policy.
- **Deliverable:** An optimization completion note and, if selected, reviewed recurring-review configuration.
- **Acceptance evidence:** Optimization remains independent of per-job completion and result evaluation. A decision to stop, defer, or reject a candidate is recorded without manufacturing a success claim.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.
