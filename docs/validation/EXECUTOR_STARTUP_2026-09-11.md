# Explicit report instructions — 2026-09-11

On macOS/arm64, the pinned Codex CLI 0.153.4 / gpt-5.5 report invocation
failed before producing model output when a private data home was nested below
a Git checkout. A synthetic, tool-free diagnostic using the same filesystem
profile reported failure loading the surrounding AGENTS.md: Operation not
permitted. `--ignore-rules` alone did not suppress project instruction discovery.

Report invocation now sets `project_doc_max_bytes=0`, as the existing conversation
adapter already did. This preserves the package-only read boundary and the
explicit pinned Skill. It does not grant access to the surrounding checkout.
Local CLI help, feature availability and the official configuration reference
were inspected; the evidence for the fix is the actual rerun below.

After the change, the same saved synthetic nine-message input produced a report
in a second attempt with all nine source lookup receipts. The job was
`waiting_input` because the synthetic mail included an ambiguous request, not
because startup failed. The first failed attempt remains preserved.

Existing `internal/feedback` and `internal/runner` tests passed; executor has no
test files. Build and affected-package vet passed. No new tests were created.
This is a macOS startup correction, not evidence of Linux model execution or
company-account access.
