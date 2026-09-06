# Feedback Tooling Checkpoint

Date: 2026-09-06. This prepares P3-01 through P3-06. MS1/P3-07 is not complete.

## Current real candidate evidence

The user-authorized [bounded Gmail trial](PHASE_04_GMAIL.md#current-bounded-live-report)
now has two actual model outputs from the same preserved input. An AI-authored
finding about operational-alert classification and summary wording supported
an inactive guide proposal and a separate experiment. The second output and
its remaining notification/priority-order issues are preserved in another
finding. These observations are not human quality judgments or proven causes.

The comparison retains both partial runs, checks matching input/request/model
conditions, and reports two unevaluated runs with `comparable: false` because
independent semantic evaluations are absent. The active digest remained
unchanged, and neither experiment advanced live production coverage. There is
no adoption, golden-set promotion or user-effort improvement claim.

This advances P3-03/P3-04/P3-05 evidence. A human-reviewed rubric, independent
confirmation cases, semantic judgments and a human decision are still needed
for the complete first feedback milestone. The following sections preserve
the earlier synthetic implementation checkpoint.

## Implemented

SQLite schema version 2 adds an index of immutable case, rubric, evaluation, feedback, finding, comparison, proposal and decision files. Cases carry their source snapshots and explicit review status; evaluations bind an attempt/result to case/rubric versions and require a judgment for every criterion, including unknown. Checking effort is nullable and never invented.

Comparison preserves both runs and all attempts, including failure and interruption. It checks input, human request context, mode, budgets, executor configuration/runtime and evaluator provenance. Candidate controls stay inactive. Experiments clone the preserved input into a distinct job. Adoption needs a comparable baseline/candidate record and an explicit decision; rejection and rollback preserve history. A decision record is a selection request; the active file pointer establishes whether publication actually succeeded.

## Checks actually run

- Built and ran focused vet on the changed feedback, store, mail, workgroup, runner and CLI packages.
- In a temporary private root, imported a draft rubric and a synthetic case explicitly labeled validation, then appended two distinct unevaluable evaluations of an intentionally failed executor preflight.
- Confirmed evaluation imports left the job's operational status unchanged.
- Stored a separate optimization finding and candidate without changing active controls.
- Created a separate candidate experiment from the same preserved source input.
- Compared both failed runs: both attempts and their unevaluable results remained in the comparison and the result was marked incomparable.
- Rejected adoption with that incomparable record, then recorded an explicit validation rejection and verified that the active digest stayed unchanged.

These are implementation walkthroughs. No generated test suite was created. No real AI output, human-approved golden expectations, useful quality improvement, real adoption or rollback has been demonstrated. The technical candidate did not claim an actual semantic improvement. Those remain necessary before MS1 can be declared complete.

Final integration review additionally rejected validation-actor adoption before any active-control mutation. Comparisons now mark unpinned model defaults as a limitation. A synthetic Gmail-origin candidate was admitted independently of its production acquisition, with matching input and semantic request digests. Seeded publication of its experimental result did not advance production source coverage or replace production history. Those publication records were deliberately synthetic checkpoints, not actual AI outcomes.
