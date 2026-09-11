# Phase 3 — Result Evaluation and the First Improvement Loop

[Master plan](../../IMPLEMENTATION_PLAN.md)

The approved modular-agent goal adds [selected review and release requirements](../validation/MOD_01_SELECTED_REVIEW_2026-09-11.md).
Its actual synthetic loop is technical evidence; it does not complete personal
quality review or a human adoption milestone in this phase.

**Status:** Real issue-to-candidate rerun and an explicitly unevaluated comparison recorded; human-decision milestone pending

Latest cross-workgroup evidence: the [2026-09-08 Jira/Skill checkpoint](../validation/JIRA_SKILL_INTEGRATION_2026-09-08.md)
includes independently evaluated Jira baseline/candidate runs and a comparable
tuning pair. The earlier Gmail candidate comparison remains unevaluated; MS1
and human acceptance are still pending. The [2026-09-10 review](STATUS_2026-09-10.md)
prepares the missing quality/adoption gate and evaluator-isolation work without
changing task status or approving a new contract.

**Goal:** Demonstrate that preserved outputs can lead to a justified change and fair revalidation, while keeping result evaluation and optimization independent.

**Entry conditions:** Phase 2 has a real executor result and its evidence. Reviewed expectations are available for the cases being evaluated.

**Exit evidence:** A real observed issue has an evaluation, candidate change, comparable reruns, regression review, and a recorded human adoption or rejection. Creating score output alone is insufficient.

**Out of scope:** Mandatory per-job grading, autonomous control promotion, a large benchmark platform, recurring optimization service, and compulsory golden-set growth.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P3-01 — Finalize the initial case expectations and rubric

- **Status:** in_progress
- **Depends on:** P2-07, P0-04; D02/D10.
- **Work:** Have the human confirm expected facts, prohibited errors, acceptable alternatives, unresolved judgments, and unacceptable omissions. Define the initial comparison baseline and sample/repeat needs from observed variability and burden.
- **Deliverable:** Versioned case expectations and an evaluation rubric.
- **Acceptance evidence:** Cases trace to source evidence and M1–M9. Expectations are not just exact prose matching; unreviewed expectations remain unreviewed. No arbitrary pass percentage is introduced.

### P3-02 — Store independent evaluation records

- **Status:** in_progress
- **Depends on:** P3-01; review the evaluation extension to D04.
- **Work:** Link evaluator/rubric/case versions, judgments, reasons, unknowns, and artifact references without changing historical completion decisions. Support human semantic judgments; AI grading remains optional and explicitly qualified.
- **Deliverable:** Evaluation persistence and a terminal evaluation entry point.
- **Acceptance evidence:** A completed job can remain operationally complete while receiving a poor evaluation. Re-evaluation adds a record instead of overwriting an earlier judgment.

### P3-03 — Produce honest comparisons and summaries

- **Status:** in_progress
- **Depends on:** P3-02.
- **Work:** Compare runs with their input, as-of time, workgroup, executor/tool, budget, and evaluator versions. Include failed attempts, absent artifacts, partial inputs, and unevaluable outcomes. Record measured time/cost only when available.
- **Deliverable:** A comparison report and provenance summary.
- **Acceptance evidence:** The denominator does not silently exclude failures or unknowns. A changed evaluation rule requires comparable rejudgment of both results or an explicit comparability limitation.

### P3-04 — Record feedback, optimization findings, and candidate changes separately

- **Status:** in_progress
- **Depends on:** P3-02.
- **Work:** Represent result feedback and optimization findings as independent records that can reference one run or several runs. A candidate names evidence, hypothesis, changed assets, expected effect, and required checks. Keep the candidate outside active controls.
- **Deliverable:** Proposal records and minimal optimization-review linkage.
- **Acceptance evidence:** A candidate may exist without adding a golden case. An optimization finding is not automatically a proven cause, an evaluation score, or an active rule.

### P3-05 — Run a controlled candidate comparison

- **Status:** in_progress
- **Depends on:** P3-03, P3-04.
- **Work:** Select an actual observed issue, edit a candidate asset version, and rerun the affected and relevant existing cases under comparable conditions. Keep tuning material separate from independent confirmation cases or report the lack of an independent set.
- **Deliverable:** Baseline/candidate bundles with a regression review.
- **Acceptance evidence:** The original bad result remains available. Future data, previous independent-run caches, and private golden answers do not leak into execution. A rejected or inconclusive candidate is a valid outcome.

### P3-06 — Implement human adoption and rollback records

- **Status:** in_progress
- **Depends on:** P3-05; reviewed active-control update contract.
- **Work:** Provide a reviewable candidate diff and validation evidence. Only an authorized human action selects the active version. Preserve rejection, adoption, and rollback history; running attempts retain their pinned version.
- **Deliverable:** A candidate adoption/rejection workflow and version-history handling.
- **Acceptance evidence:** Executor/evaluator paths cannot promote controls. A later run uses an adopted version; rollback does not rewrite past packages or evaluations.

### P3-07 — Close the first useful feedback milestone

- **Status:** not_started
- **Depends on:** P3-06; D10.
- **Work:** Review one full issue-to-decision chain and the usefulness of the report. Record user checking/correction effort, unresolved quality issues, and the limits of the dataset. Keep deep optimization deferred.
- **Deliverable:** The MS1 completion note and a prioritized live-mail readiness list.
- **Acceptance evidence:** The chain includes execution, preserved evidence, evaluation, candidate, comparison, and human decision. Synthetic success is not labeled live-work validation; optimization remains an independent optional review axis.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.

Current evidence: [Feedback tooling checkpoint](../validation/PHASE_03_FEEDBACK.md).

The actual Gmail pilot produced an initial report, an AI-authored finding, a
revised inactive guide and a second real run of the identical input. Their
comparison remains unevaluated and cannot justify adoption. No human acceptance
or permanent workgroup change has been inferred from the trial.
