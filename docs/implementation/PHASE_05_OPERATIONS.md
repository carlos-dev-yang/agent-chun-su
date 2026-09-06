# Phase 5 — Repeated Local Operation and Recovery

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** Operations preparation in progress; no background activation or live reliability claim  
**Goal:** Make useful manual mail reporting repeatable without losing work, silently duplicating delivery, or requiring constant supervision.

**Entry conditions:** P4-09 establishes a useful manual pilot. The user chooses the operating policy before background activation.

**Exit evidence:** Queue ownership, waits, cancellation, restart, missed schedules, data recovery, and local delivery have explicit demonstrated behavior.

**Out of scope:** Distributed leases, multiple hosts, mandatory external notifications, root/system services, and exactly-once execution claims.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P5-01 — Specify operating and intervention policies

- **Status:** not_started
- **Depends on:** P4-09; D07.
- **Work:** Define schedule/timezone, overlap handling, named retry/time limits, missed-run catch-up, user-question handling, partial report availability, retention, and destination policy. Prepare a concrete terminal experience for resolving blocked work.
- **Deliverable:** A local-operations policy and runbook draft.
- **Acceptance evidence:** The policy describes what happens during sleep, logout, provider outage, missing authentication, and no new input. External delivery remains absent unless separately selected and authorized.

### P5-02 — Complete durable queue ownership and restart recovery

- **Status:** in_progress
- **Depends on:** P5-01, P1-04.
- **Work:** Add execution eligibility, one-owner coordination, attempt fencing, and interrupted-work reconciliation. Keep transactions short and process/network activity outside DB write transactions. Treat in-memory notifications as wakeups only.
- **Deliverable:** A durable local queue and restart reconciler.
- **Acceptance evidence:** Reopening a pending or running job after interruption preserves work and rejects stale results. Recovery checks process/attempt identity before cleanup and does not blindly kill an unrelated reused process ID.

### P5-03 — Implement stage-specific retries, waits, and resume

- **Status:** in_progress
- **Depends on:** P5-02, P2-06.
- **Work:** Distinguish provider retry, executor retry, user input, authentication repair, missing evidence, and delivery retry. Record budgets and next eligible conditions; terminal answers resume the correct request and stage.
- **Deliverable:** Queue-aware retry/cancel/resolve commands and blocked-reason presentation.
- **Acceptance evidence:** A repaired connection or answered question resumes only eligible work. Failed report delivery does not automatically rerun the AI, and uncertain effects are inspected before any repeated action.

### P5-04 — Connect CLI and service through a protected management channel

- **Status:** in_progress
- **Depends on:** P5-02.
- **Work:** Add a user-local management channel and single-owner behavior. If the service owns state, CLI operations go through it rather than mutating the database concurrently. Enforce the executor boundary around this channel.
- **Deliverable:** A local management interface and service-mode entry point.
- **Acceptance evidence:** Manual and service modes cannot become competing owners. User-only socket permissions alone are not presented as sufficient separation from a same-user executor.

### P5-05 — Provide optional macOS user-service lifecycle

- **Status:** not_started
- **Depends on:** P5-04.
- **Work:** Implement install/start/status/stop/remove for the selected user LaunchAgent. Use discovered executable paths and explicit environment/configuration; do not rely on interactive shell startup or the project working directory. Activate only under the selected operating scope.
- **Deliverable:** Terminal service management and generated per-user service configuration.
- **Acceptance evidence:** Starting/stopping/removing the service is demonstrated without root privileges. Login-session and sleep limitations are documented; removing a service preserves user data.

### P5-06 — Add scheduling and deterministic catch-up policy

- **Status:** not_started
- **Depends on:** P5-05, P5-01.
- **Work:** Persist schedule definitions and next eligibility; define ownership between launchd process management and Chun-su job scheduling. Implement the chosen time, overlap, and catch-up semantics without silently creating historical reports or duplicate work.
- **Deliverable:** A scheduler and terminal schedule configuration.
- **Acceptance evidence:** Sleep/wake, restart, timezone changes, missed intervals, and an already-running job follow the recorded policy. Scheduling does not advance source-processing state by itself.

### P5-07 — Make result availability and delivery recovery explicit

- **Status:** in_progress
- **Depends on:** P5-03.
- **Work:** Finalize local report pointers and stage-specific delivery records. If an external channel is later selected, prepare and authorize that adapter separately, including deduplication and uncertain-result recovery.
- **Deliverable:** Durable local report availability and delivery-state inspection.
- **Acceptance evidence:** A crash during final presentation can recover the same collected report without rerunning analysis. Generated, available, sent, and acknowledged states are not conflated where those states apply.

### P5-08 — Provide consistent backup, restore, and retention operation

- **Status:** in_progress
- **Depends on:** P5-02, P4-02.
- **Work:** Quiesce relevant writes and preserve DB, referenced artifacts, controls, and non-secret configuration consistently. Exclude secrets, transient files, and backup recursion. Restore to an explicit separate data root; apply the chosen retention policy with traceable limitations.
- **Deliverable:** Backup/restore commands and an operational retention runbook.
- **Acceptance evidence:** A restored bundle resolves its references and detects incompatible schema versions. A live WAL database is not backed up by copying its main file alone. Credentials require reconnection; retention does not silently invalidate claimed reproducibility.

### P5-09 — Demonstrate repeated operation and close MS3

- **Status:** not_started
- **Depends on:** P5-03, P5-06, P5-07, P5-08.
- **Work:** Run bounded restart, cancellation, authentication-loss, provider-failure, missed-schedule, and restore walkthroughs. Measure core resource use separately from the executor and record actual user intervention.
- **Deliverable:** The MS3 operations evidence note and unresolved limitations.
- **Acceptance evidence:** The repeated workflow preserves pending work, reports failures honestly, and needs intervention only where policy requires it. Checks actually run are distinct from unverified long-duration reliability claims.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.

Current evidence: [Local operations checkpoints](../validation/PHASE_05_OPERATIONS.md).
