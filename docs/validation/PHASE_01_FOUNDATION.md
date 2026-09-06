# Phase 1 Foundation Checkpoint

Date: 2026-09-06. This is a partial phase checkpoint, not a real executor or Gmail result.

## Implemented

P1-01, P1-02, P1-03 and P1-06: Go CLI, private application root, repeatable setup, explicit configuration and schema versions, SQLite jobs/attempts/events/artifacts, input snapshots, queued cancellation and persisted inspection. The installed binary reports SQLite 3.53.4 and Go 1.26.8. No external database service is needed.

P1-04/P1-05/P1-07 continue with real attempt execution. Their lifecycle and safe artifact primitives exist, but process termination, stale-result rejection and crash recovery need the adapter walkthrough.

## Checks actually run

- Built `./cmd/chunsu`; formatted changed Go packages; ran `go vet` on the six new command/foundation packages.
- Fresh setup and repeated setup in a temporary private root preserved configuration and database contents.
- Queued a synthetic JSON input, edited the original, reopened in a separate process, and verified that the stored artifact digest still matched the original snapshot.
- Listed jobs, inspected artifacts/events, and cancelled a queued job across separate CLI processes.
- Rejected a symbolic-link input.
- Held the writer lock in a separate process: configuration changes timed out, while read-only job inspection still worked.
- Seeded a future database version in the temporary database: setup refused it without resetting the database.

No test files or generated test suite were created. Runtime data and build outputs are excluded from Git.

## Not yet checked and limitations

No AI invocation, real account connection, semantic evaluation, actual worker crash, interrupted artifact publication, or cross-platform runtime validation occurred. macOS and Linux lock implementations compile targets; Windows deliberately returns an unsupported-platform diagnostic. UTC is the explicit initial timezone when the host does not expose an IANA timezone; set the desired timezone before reviewing time-sensitive mail.

Artifact publication precedes database insertion. A crash may leave an unreferenced file; such a file must never count as a successful result. Keep orphaned content for inspection until an explicit retention operation. Current recovery commands record interrupted work for review; the executor stage must add process reconciliation before using them for live workers.
