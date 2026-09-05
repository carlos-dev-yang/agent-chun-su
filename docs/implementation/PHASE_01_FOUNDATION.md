# Phase 1 — Minimal Go CLI and Durable Records

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** Not started  
**Goal:** Create the smallest runnable program that can preserve requests, attempts, artifacts, and failures for the upcoming mail feedback loop.

**Entry conditions:** P0-08 is complete for foundation decisions. No live account or selected model is required.

**Exit evidence:** A local request and artifact record can be created, inspected, reopened, and recovered without a daemon or external service.

**Out of scope:** Scheduled jobs, multi-worker scheduling, production connectors, generalized workflow engines, and a web UI.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P1-01 — Create the Go module and CLI shell

- **Status:** not_started
- **Depends on:** P0-08.
- **Work:** Create the selected module and a narrow entry point with version/help and clear error output. Add source/runtime ignore rules and build instructions. Keep module and release metadata configurable.
- **Deliverable:** `go.mod`, dependency lock information, `cmd/chunsu/`, minimal CLI code, and a repository README.
- **Acceptance evidence:** The program builds and displays help/version on the initial host. No unrelated code, generated test suite, or invented remote is introduced.

### P1-02 — Implement local paths and idempotent setup

- **Status:** not_started
- **Depends on:** P1-01; foundation portion of D04.
- **Work:** Resolve OS user paths plus an explicit override. Create non-secret configuration and runtime directories without replacing existing controls or records. Use an explicit temporary data root for validation.
- **Deliverable:** Path/configuration handling and a first `setup` command; command spelling follows the reviewed CLI contract.
- **Acceptance evidence:** A fresh setup and a repeated setup preserve user data. Invalid configuration fails clearly, and account credentials are not required for saved-input mode.

### P1-03 — Open SQLite and initialize its versioned schema

- **Status:** not_started
- **Depends on:** P1-02; reviewed storage contract from P0-07.
- **Work:** Implement embedded DB access and the minimum schema for jobs, attempts, events, and artifact references. Configure foreign keys, durability, local WAL use, and named lock-wait limits. Avoid holding transactions during external work.
- **Deliverable:** The SQLite store and initial migration with schema-version checks.
- **Acceptance evidence:** A fresh DB opens and persists data after process exit. A second initialization preserves it; unsupported schema versions produce a clear diagnostic. The app reports its actual SQLite engine version.

### P1-04 — Persist minimal lifecycle transitions

- **Status:** not_started
- **Depends on:** P1-03.
- **Work:** Record request admission and attempt start/finish/cancel/wait facts atomically where required. Preserve request and attempt identities, initial failures, and retry ancestry. Use one controller owner for this stage.
- **Deliverable:** The minimal lifecycle service and event records.
- **Acceptance evidence:** An interrupted attempt is identifiable after restart and cannot silently become complete. An old attempt cannot overwrite the state of a newer attempt.

### P1-05 — Store and collect artifacts safely

- **Status:** not_started
- **Depends on:** P1-03; foundation artifact contract accepted in P0-08.
- **Work:** Write through controller-owned staging, finalize an artifact, then record its reference. Constrain accepted paths, symlinks, sizes, and filenames with named policy. Preserve provenance and content identity.
- **Deliverable:** Artifact storage and collection helpers, without a general object-storage abstraction.
- **Acceptance evidence:** Manual checks cover an interrupted write and an out-of-scope path. No success record refers to a missing final artifact; orphaned staging content has a documented recovery disposition.

### P1-06 — Expose inspection and mode-aware diagnostics

- **Status:** not_started
- **Depends on:** P1-04, P1-05.
- **Work:** Add narrow job/status/log/artifact inspection and `doctor` output. Separate human-readable output from machine-readable output under a reviewed option. Avoid logging content or secrets by default.
- **Deliverable:** Inspection commands and diagnostics.
- **Acceptance evidence:** A persisted sample record is inspectable after restart. Saved-input readiness does not require a mail account, Go compiler in release mode, or an active service.

### P1-07 — Demonstrate the foundation checkpoint

- **Status:** not_started
- **Depends on:** P1-06.
- **Work:** Build and format the affected code, perform a local setup/persist/reopen/interruption walkthrough in a temporary data root, and run relevant existing checks if present. Record unimplemented capabilities honestly.
- **Deliverable:** A Phase 1 completion note under `docs/validation/`.
- **Acceptance evidence:** The walkthrough shows durable state and artifact behavior. It is explicitly not a real executor run, a quality evaluation, or evidence of full isolation.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.
