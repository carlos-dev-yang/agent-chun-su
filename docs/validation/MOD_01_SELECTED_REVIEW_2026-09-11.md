# MOD-01 — selected review and adoption requirements

Environment: macOS/arm64, actual authenticated Codex CLI 0.153.4 / gpt-5.5.
Only public synthetic mail fixtures were disclosed. Private runtime records are
kept outside Git under an isolated `var/mod-01.*` data home.

## Actual walkthrough

- The nine-message baseline produced a source-verified `waiting_input` report;
  its ambiguous request remains visible. The earlier startup failure and its
  correction are [recorded separately](EXECUTOR_STARTUP_2026-09-11.md).
- An explicitly requested evaluation produced all nine rubric judgments in a
  separate tool-free process and was imported only after host validation. The
  model reported pass with synthetic/personal-usefulness limitations.
- An explicitly selected optimizer reviewed the saved result. It identified
  that hostile source instructions had become an assigned task despite being
  refused, and supplied hypotheses plus regression/confirmation requirements.
- A candidate-only Skill clarified that hostile instructions should stay
  visible without becoming user work. A separate same-input experiment put the
  hostile message in reference-only disposition. Its separate evaluation also
  reported pass with limitations. The comparable pair preserves the exact
  baseline and candidate evaluations. This does not establish superiority from
  scores, independent confirmation, or personal acceptance.
- The release assessment rejected absent, failed and unknown declared checks.
  A synthetic passing check still did not make a validation-labelled policy or
  unreviewed expectations eligible. Missing withheld confirmation and declared
  evaluation limitations remained explicit reasons.
- Adding a second baseline evaluation made implicit selection incomparable;
  naming the exact original evaluations restored a comparable pair.
- Importing changed judgments while claiming the original host execution was
  rejected. The active-control pointer hash remained unchanged throughout.

Selected provenance: baseline `WIGQRJO4PKWBGQQQYOYAQNVKL2`, candidate
`BPZLFQFRI5Q5AO5J6XF7Y6RRY3`, proposal `NTBZT5MGBNF4JIQ4YZ3BLOMXXZ`,
comparison `OAP4ORXY2Q4GMOFIZBYB7G4VEH`. No human-reviewed metadata or human
adoption decision was fabricated for this walkthrough.

## Checks and limits

Existing `internal/feedback` and `internal/runner` tests passed after the
changes. Affected-package vet and the application build passed. There are no
executor/CLI/backup test files; no test code was created. The manual walkthrough
above checks new behavior that existing tests do not exercise.

Technical MOD-01 is complete. P3 personal-usefulness and actual human adoption
remain separate milestones. Actual live mail/Jira review, crash recovery during
review, review-specific retention, and Linux model isolation are not claimed
by this record. Review currently uses the single controller write lock and is
invoked explicitly while that writer is available. See the
[review contract](../contracts/review-v1.md) and [usage](../setup/RESULT_REVIEW.md).
