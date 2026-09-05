# Phase 0 — Decisions and First Mail Contracts

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** P0-01 complete; remaining tasks not started  
**Goal:** Make foundation work startable while preparing the concrete decisions needed for the first useful mail report.

**Entry conditions:** The user has selected Go + SQLite. Current principles and historical proposals are available.

**Exit evidence:** P0-08 records which foundation contracts can be implemented. Executor, personal-semantic, and live-provider decisions only block their dependent work.

**Out of scope:** Application implementation, live account access, production datasets, service registration, and a comprehensive universal schema.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P0-01 — Reconcile the current documentation

- **Status:** complete
- **Depends on:** None.
- **Work:** Record accepted principles, the selected stack, superseded assumptions, and the phase sequence. Preserve the old implementation plan verbatim.
- **Deliverable:** The root implementation plan, phase documents, and archived original.
- **Acceptance evidence:** Current links and task references resolve; the archive matches the old file. This is documentation completion only.

### P0-02 — Record the smallest build and dependency baseline

- **Status:** not_started
- **Depends on:** P0-01.
- **Work:** Recheck the current checkout and toolchain. Select a supported Go patch, a module identity without inventing a remote, the CLI library, and SQLite driver. Inspect native build requirements, including future secret-store integration.
- **Deliverable:** A build decision under `docs/decisions/`, with exact versions to use when implementation starts.
- **Acceptance evidence:** The proposed core has a build path for the initial Mac and does not require an end-user Go compiler or SQLite CLI. Extra runtime dependencies are explicitly listed.

### P0-03 — Draft personal mail semantics with concrete examples

- **Status:** not_started
- **Depends on:** P0-01; D02 is developed here.
- **Work:** Draft inclusion/exclusion examples for the three categories, user role, importance, spam, interview states, previous history, and delta versus outstanding-action reporting. Distinguish source channel from business meaning.
- **Deliverable:** A mail behavior brief and sample report in `docs/contracts/`.
- **Acceptance evidence:** Each open decision has a concrete example and a proposed behavior. No unknown personal preference is represented as confirmed.

### P0-04 — Specify saved cases and expectation ownership

- **Status:** not_started
- **Depends on:** P0-03; D02/D03 for personal or real-data expectations.
- **Work:** Define source IDs, as-of time, timezone, allowed history and lookups, data origin, required facts, prohibited claims, acceptable variations, and unknowns. Include a sequence with a schedule proposal, change, and a later run with no new mail.
- **Deliverable:** A case catalog and human review template; executable evaluation data is a later deliverable.
- **Acceptance evidence:** Coverage includes all mail requirements M1–M9 from the readiness review. Future messages are excluded from earlier snapshots; synthetic examples are distinguishable from validated personal cases.

### P0-05 — Choose and assess the first executor adapter

- **Status:** not_started
- **Depends on:** P0-02; D01/D03 are developed here.
- **Work:** Compare only concrete available executor choices against instruction delivery, scoped tools, structured output, cancellation, authentication, observable traces, and filesystem/network restrictions. Prepare a recommendation without making a paid or real-data model call.
- **Deliverable:** An executor capability and data-boundary decision under `docs/decisions/`.
- **Acceptance evidence:** Documented support and unverified assumptions are separate. A regular child process is not claimed to isolate controls, golden answers, secrets, or management sockets.

### P0-06 — Draft request, package, result, and evidence contracts

- **Status:** not_started
- **Depends on:** P0-03, P0-04; use P0-05 findings when available; D04.
- **Work:** Specify target inputs versus reference history, many-to-many source-to-business-item mapping, coverage, partial results, version pinning, late lookups, and the boundary between public criteria and private expectations.
- **Deliverable:** Minimal contract documents and illustrative examples; no SQL migration or public API is created.
- **Acceptance evidence:** Representative cases can be expressed without forcing one message to equal one task. Permission, validity, completion, delivery, and semantic judgment remain separate.

### P0-07 — Draft local state, configuration, and recovery semantics

- **Status:** not_started
- **Depends on:** P0-02; foundation portion of D04.
- **Work:** Define job versus attempt, immutable collected evidence, waiting reasons, cancellation, retry versus improvement experiment, artifact ownership, configuration precedence, named limits, and version changes. Model only the states needed by the first slice. Coordinate identifiers with P0-06 as it develops, without making unresolved personal mail semantics a prerequisite for foundation storage.
- **Deliverable:** A small lifecycle/ownership document and foundation storage proposal.
- **Acceptance evidence:** A crash, late result, invalid artifact, no-input run, and pending user answer each have an explicit interpretation. Nothing treats in-memory queues or AI self-report as authoritative state.

### P0-08 — Record foundation readiness and remaining first-run blockers

- **Status:** not_started
- **Depends on:** P0-02, P0-07; the foundation subset of D04.
- **Work:** Review the concrete foundation proposal before schema/interface implementation where the direction is unresolved. Record accepted portions and keep personal, executor, and provider decisions attached to later tasks. Do not reopen Go + SQLite.
- **Deliverable:** A decision log and a ready list for Phase 1.
- **Acceptance evidence:** Foundation changes have an established direction and completion evidence. P0-03 through P0-06 may still have clearly named first-run blockers; unrelated CLI/storage work can proceed.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.
