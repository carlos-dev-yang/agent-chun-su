# Evaluation records

The initial rubric is a draft, not a human-approved golden standard. Importing it stores a version; later judgments reference that immutable record ID. A case record includes its source snapshot, evidence-backed expectations, prohibited errors, acceptable alternatives, review status and limitations. Case snapshots are validated against the evaluated job. Private expected answers never enter execution packages.

`mail-filtering.case.json` contains source-grounded expectations for the synthetic
filtering fixture. Its review status is `validation`, not personal approval. It
was stored after the first gate failure, and is tuning material rather than an
independent holdout. The host-only [evaluation procedure](../../docs/skills/golden-evaluation.md)
and [loop evidence](../../docs/validation/EXECUTION_GOLDEN_LOOP_2026-09-07.md)
separate operational failures, semantic judgments and remaining work.

`mail-history.case.json` and `mail-partial.case.json` were frozen before their
gate-loop executions. They cover source-linked history/current schedule changes
and declared acquisition/body/attachment gaps respectively. Both remain
synthetic validation material, with no human approval or broader coverage claim.
`mail-correction.case.json` adds a minimal previous-report/human-correction
scenario over `examples/mail/prior-correction.json`, also frozen before execution.
Its named human correction is synthetic source data, not a real user approval.

Evaluation imports identify the attempt and preserved result artifact, rubric and case IDs, reviewer identity/type, one judgment for every rubric criterion (including unknown), and optional measured checking time. A missing result is unevaluable. Re-evaluation appends a record. Findings, result feedback, comparisons, proposals and decisions are separate record kinds.

An adoption requires a comparable baseline/candidate record and an explicit human command. Rejection is a valid outcome. A decision record records a selection request; the active pointer, including its decision ID, establishes whether adoption actually took effect. Failed pointer publication must be inspected and retried explicitly. A rollback selects a preserved version without changing earlier runs.

Use `feedback --help`, `workgroup --help` and `experiment --help` for the terminal workflow. Do not label implementation-generated expectations or validation decisions as the user's personal approval.
