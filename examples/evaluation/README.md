# Evaluation records

The initial rubric is a draft, not a human-approved golden standard. Importing it stores a version; later judgments reference that immutable record ID. A case record includes its source snapshot, evidence-backed expectations, prohibited errors, acceptable alternatives, review status and limitations. Case snapshots are validated against the evaluated job. Private expected answers never enter execution packages.

Evaluation imports identify the attempt and preserved result artifact, rubric and case IDs, reviewer identity/type, one judgment for every rubric criterion (including unknown), and optional measured checking time. A missing result is unevaluable. Re-evaluation appends a record. Findings, result feedback, comparisons, proposals and decisions are separate record kinds.

An adoption requires a comparable baseline/candidate record and an explicit human command. Rejection is a valid outcome. A decision record records a selection request; the active pointer, including its decision ID, establishes whether adoption actually took effect. Failed pointer publication must be inspected and retried explicitly. A rollback selects a preserved version without changing earlier runs.

Use `feedback --help`, `workgroup --help` and `experiment --help` for the terminal workflow. Do not label implementation-generated expectations or validation decisions as the user's personal approval.
