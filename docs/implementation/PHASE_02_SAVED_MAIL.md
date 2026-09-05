# Phase 2 — Saved-Input Mail Execution

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** Not started  
**Goal:** Use one real executor to produce a source-backed local mail report from a bounded saved dataset.

**Entry conditions:** Phase 1 is usable. P0-03 through P0-06 and D01–D04 are resolved for the first execution scope; synthetic demonstration limits may be explicitly accepted.

**Exit evidence:** A saved-input request produces a collected, validated report with source dispositions and truthful operational status. The initial result and failures remain available for Phase 3.

**Out of scope:** Live mailbox access, automatic scheduling, external delivery, additional executor providers, and automatic optimization.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P2-01 — Create the mail workgroup and saved-input reader

- **Status:** not_started
- **Depends on:** P1-07, P0-03, P0-04, P0-06; D02/D03/D04.
- **Work:** Create the reviewed guide, public examples, result schema, and versioned saved-input format. Distinguish message, thread, business item, source disposition, schedule state, prior interpretation, and current source evidence.
- **Deliverable:** `workgroups/mail-review/` and a saved-input reader; labeled synthetic examples where needed.
- **Acceptance evidence:** Representative many-to-many, classification, schedule, exclusion, and partial-input cases are expressible. Private expected answers are stored outside the execution package.

### P2-02 — Build pinned execution packages

- **Status:** not_started
- **Depends on:** P2-01, P1-05.
- **Work:** Resolve one active workgroup version, scope, input snapshot, tool contract, and named budget. Generate the initial instructions and public completion requirements from the pinned declarations; preserve manifest/provenance.
- **Deliverable:** Package preparation with an explicit initial instruction payload and artifact manifest.
- **Acceptance evidence:** Editing an active source file after packaging does not silently change the attempt. Target sources and reference-only history stay distinguishable; hidden evaluation answers are absent.

### P2-03 — Implement the first adapter and its enforceable boundary

- **Status:** not_started
- **Depends on:** P2-02, P0-05; D01/D03.
- **Work:** Implement only the selected invocation method, input/output channels, termination, and capability restrictions. Prevent executor access to active-control writes, private evaluation answers, unrelated runs, secrets, and the management channel. If available isolation cannot enforce the scope, record the blocker and revise the concrete design.
- **Deliverable:** One executor adapter and a documented, verifiable boundary.
- **Acceptance evidence:** A narrowly scoped synthetic invocation verifies instruction delivery, output collection, and timeout/cancellation. Attempted access outside the permitted scope is checked; a directory layout alone is not accepted as proof.

### P2-04 — Expose bounded saved-source lookup tools

- **Status:** not_started
- **Depends on:** P2-03.
- **Work:** Allow the executor to choose relevant saved messages/threads and explicitly supported material. Bind tool calls to the attempt and scope, check revocation, and record actual responses, missing/truncated content, and errors. Use recorded data for this phase.
- **Deliverable:** The first gateway transport and saved-source tool implementation.
- **Acceptance evidence:** An allowed lookup produces host evidence. Out-of-scope lookup, cancelled access, and unavailable source content have distinct outcomes. No live API credentials or unrestricted HTTP tool is exposed.

### P2-05 — Validate and present the local report

- **Status:** not_started
- **Depends on:** P2-04.
- **Work:** End the attempt's write access before collecting structured output and rendering local Markdown; reject stale-attempt output. Check schema, reference existence, per-target dispositions, and known acquisition failures; then apply the declared complete/partial/wait decision. Separate generated, validated, and locally available states.
- **Deliverable:** Mail validation, completion handling, report rendering, and a manual run command.
- **Acceptance evidence:** A valid structure with an unsupported source reference fails the relevant check. Empty successful collection and failed collection are distinguishable. Semantic correctness is not claimed merely because validation passed.

### P2-06 — Handle bounded failures and user clarification

- **Status:** not_started
- **Depends on:** P2-05.
- **Work:** Implement per-stage error classification and named retry budgets for transient execution/contract failures. Record a blocked question and accept a terminal answer tied to the original request. Resume from an explicit checkpoint without overwriting attempts.
- **Deliverable:** Manual cancellation, retry, and clarification/resume behavior.
- **Acceptance evidence:** Malformed output, timeout, nonzero exit, unsupported input, and a pending answer leave usable evidence. Retry budgets terminate; semantic evaluation scores do not trigger an unbounded operational retry loop.

### P2-07 — Demonstrate one traceable saved-input mail run

- **Status:** not_started
- **Depends on:** P2-06.
- **Work:** Run representative reviewed cases with the selected executor and preserve package, lookups, outputs, validation, and completion evidence. Include sequence/time-boundary and no-new-input behavior appropriate to the agreed report semantics.
- **Deliverable:** A Phase 2 report bundle and completion note.
- **Acceptance evidence:** The report addresses the three categories, reasons, importance, history, direct schedule impact, and exclusions. Source mapping is auditable. Synthetic evidence is labeled, and remaining semantic problems are inputs to Phase 3 rather than erased.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.
