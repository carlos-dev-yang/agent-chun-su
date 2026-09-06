# Local Operations Checkpoints

Date: 2026-09-06. Phase 5/MS3 is not complete. No background service or schedule has been activated.

## Publication and backup checkpoint — P5-07/P5-08

Implemented verified report inspection, presentation recovery from preserved output, idempotent report reuse, consistent SQLite/file backup, manifest verification, and restore into a new private root. Restore disables connections, gives them fresh unused credential references, and leaves pending work for explicit review. It never reads or copies Keychain values. Candidate controls and private evaluation records are preserved as local files; socket/lock files, backup recursion and incomplete staging files are excluded.

Checks actually run:

- Built the CLI and ran focused vet on changed backup, file, runner, store, CLI and configuration packages.
- Seeded a synthetic publication checkpoint with explicitly labeled synthetic executor metadata; rendering and repeated publication used the same attempt/report artifact without invoking an executor.
- Seeded a failure after artifact publication and before final job status; publication recovery reused the artifact and finished the recorded presentation stage.
- Kept committed SQLite data in WAL with a separate open connection. The backup's database image retained that committed event without copying WAL sidecars.
- Restored into a separate root and resolved the original report and metadata references.
- Refused an existing restore destination and rejected a deliberately damaged backup file.

These are bounded process/persistence walkthroughs with synthetic data. No actual AI publication failure, restored live account, successful Keychain operation, power-loss durability, long-running service behavior or live operational usefulness has been demonstrated. No generated test suite was added.

## Scheduling, management and retention checkpoint — P5-01/P5-04–06/P5-08

Added disabled-by-default elapsed-interval schedules, durable occurrence/acquisition identities, missed-interval coalescing, bounded collection retries, blocked-occurrence intervention and persisted admission pause. The foreground worker integrates scheduling; supported management commands reach its sole owner. Added reviewable macOS LaunchAgent generation and explicit lifecycle commands. Added reviewed run-content retention with stale-plan rejection, interrupted-application state, purged tombstones and idempotent completion. Schema 4 is an additive local migration. Restore now disables schedules and pauses admission as well as disabling connections.

Checks actually run on synthetic data in a separate short temporary data root:

- Fresh setup reported schema 4; schedule creation was disabled and enabling a disabled synthetic connection was refused.
- Seeded a due occurrence three intervals in the past with a disabled connection. The worker recorded one occurrence with three missed intervals, no credential lookup/collection attempt, and `waiting_auth`. Restart did not create another occurrence; explicit retry reused its occurrence and reserved acquisition IDs.
- Started a paused foreground worker with an intentionally unavailable executor path. The protected socket had mode 0600; status, cancellation and pause/unpause reached the active owner. No model was invoked and shutdown completed.
- Retention refused missing confirmation and a changed run directory. Applying the reviewed plan removed only the selected synthetic run, retained a purged job, and a repeated application reused completed status.
- Parsed the rendered plist with the system Python plist parser; verified absolute program/data arguments and disabled login/crash activation. The target LaunchAgent did not exist and was not installed or started.
- Backed up and restored the purged run metadata plus synthetic schedule/connection. Reference verification succeeded, schedules and connections were disabled, admission was paused, and restored secret references differed from the original.
- Built the CLI and ran focused vet on the changed operations and directly related packages.

Not run: actual scheduled Gmail retrieval, provider retry timing, full executor cancellation, launchd start/stop/remove, real sleep/wake or DST transitions, process/power interruption during physical deletion, secure erasure, and long-duration resource/reliability measurements. UTC interval arithmetic and a missed due timestamp are not a real sleep/wake test. The [runbook](../setup/LOCAL_OPERATIONS.md) is an implementation policy draft, not the user's approval to activate background work.

## Final integration review checkpoint — P5-02/P5-03/P5-09

A seeded job with an exhausted attempt budget moved from queued to waiting_input without launching an executor; it no longer stops the worker as an unclassified queue error. A deliberately mismatched process-start identity refused recovery and left an unrelated self-created process group alive. Compatible schema-3-shaped synthetic data upgraded to schema 4 without losing jobs. Restore reset the newly added live-mail disclosure approval as well as other active controls.

One paused foreground-core sample reported RSS 23,168 KiB and CPU 0.8% about 0.4 seconds after startup. No executor was running. This is one short observation, not a peak-memory figure, steady-state CPU measurement or long-duration benchmark. The native core built with CGO disabled, and whole-module vet passed for this cross-cutting implementation. No new test files were created.
