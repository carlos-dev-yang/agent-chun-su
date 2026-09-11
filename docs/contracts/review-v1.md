# Selected-result review and release requirements v1

Review is optional and explicitly requested. Report completion never schedules
an AI grade. `review evaluate` selects one saved job, matching case, rubric and
independent evaluator Skill by immutable record ID. `review analyze` selects a
set of jobs, a bounded question and versioned analyzer/optimizer criteria. It
does not replay those jobs. These paths use a separate ephemeral model process
with no native tools or source gateway. They receive only selected evidence and
have no authority to change task state or active controls.

## Criteria and evidence

Skills, cases, rubrics, review criteria and release policies are immutable
records. Updated content creates a new ID. Cases can declare `purpose` as
`tuning`, `regression` or `confirmation` and `exposure` as `development` or
`withheld`. These declarations record provenance; software cannot prove that a
human has never seen a case. `draft` and `validation` are not human acceptance.

The host records a review request before starting, with exact source attempt,
input/result artifacts, selected criteria and prompt/schema/Skill hashes.
Source disclosure uses the existing mail/Jira policy. Changed or revoked live
connections and missing current model disclosure approval reject review.
Jira disclosure uses the report field projection, not the full acquisition.
Lookup receipts and structural validation accompany the result so the reviewer
can distinguish actual retrieval from a model's claim.

Results and execution receipts are preserved under `reviews/`, including failed
and invalid responses. A completed process is not necessarily an accepted
evaluation. The host validates schema, criterion coverage, source references,
attempt/result ownership and generated-result identity before importing an AI
evaluation. Imported external AI judgments without a host execution remain
possible, but are distinguishable and may be excluded by release policy.

An analyzer returns sourced observations; optimizer criteria additionally
require hypotheses and checks. Findings cannot activate a candidate. Review
records are included in backup. Job-content purge does not erase historical
evaluation/review copies; review retention must be managed separately before
using sensitive data. Interrupted requests are never automatically replayed.

## Explicit comparison and adoption

When a job has multiple evaluations, comparison requires an explicit selection.
A sole evaluation is unambiguous. Neither newest evaluation nor a favorable
evaluation is silently chosen. Baseline and candidate still need matching
inputs, cases, rubric, evaluator Skill and compatible execution evidence.

Adoption additionally requires an explicitly selected, human-reviewed
`release_policy` and one selected `check` result for every proposal check.
The policy names required passing criteria, allowed result outcomes and job
states, and controls reviewed expectations, withheld confirmation provenance,
isolated AI evidence, regression rejection and unresolved limitations. There
is no built-in numerical quality score or default adoption policy.

`workgroup assess` saves eligibility and every unmet requirement without
changing controls. Missing, failed or unknown check results are not passes.
`workgroup decide --action adopt` repeats assessment against current job and
active-control state, records the assessment ID, and requires the existing
explicit human decision. A passing assessment alone never adopts. Rollback
retains the existing explicit human path. Old comparisons lacking evaluation
selection must be recreated to adopt under this contract.

This is a trusted single-owner controller contract. `reviewer` and `actor`
describe attribution; arbitrary strings are not organizational authentication.
Task/review model processes cannot call these management commands. A future
multi-user endpoint must derive authority from an authenticated principal.
