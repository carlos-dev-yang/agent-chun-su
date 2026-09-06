# Jira ingestion checkpoint

Date: 2026-09-06. Scope: the user-approved saved collection, Jira-specific
normalization and raw/normalized preservation layer. All input in this
checkpoint is synthetic. Real Jira connection and AI reporting are deferred.

## Bounded unit status

| Unit | State and evidence |
|---|---|
| P6-01a | Complete: concrete collection boundary reviewed and approved by the user |
| P6-02a | Complete: replaceable reader, saved Cloud/v2-shaped responses and disconnected live mode |
| P6-03a | Complete for the documented formats: deterministic facts, explicit mappings, unknown/partial values and provenance |
| P6-03b | Complete: private raw/snapshot files, existing SQLite index, bounded collection/resume, immutable remapping and inspection |
| P6-03c | Complete for this storage seam: mixed legacy Gmail/Jira listing, backup verification and separate-root restore |
| P6-02b | Deferred: live provider choice, HTTP adapter and authentication lifecycle |
| P6-04 onward | Not started: Jira guide/result contract, executor tools/dispatch and evaluation |

The parent P6-01/P6-02/P6-03 tasks remain in progress. Full Phase 6, MS4 and the
mail usefulness milestones are not complete.

## Behavior and artifacts

- `internal/jira/reader.go` isolates saved response retrieval and pagination.
- `internal/jira/model.go` and `normalize.go` define the versioned Jira evidence
  model and deterministic mapping, without network or secret access.
- `internal/jira/acquire.go` preserves source/raw/snapshot files and checkpoints
  through the existing acquisition store, without a DB migration.
- `internal/cli/jira.go` exposes status, inspect, collect, resume, renormalize,
  acquisitions, show and exact source inspection.
- Provider-aware listing in the store and Gmail CLI prevents Jira records from
  appearing as Gmail acquisitions. Existing Gmail checkpoint files are unchanged.
- [Synthetic examples](../../examples/jira/), [contract](../contracts/jira-evidence.md)
  and [terminal guide](../setup/JIRA_INGESTION.md) document the available surface.

## Checks actually run

The native CLI was built with CGO disabled. Focused Go vet covered Jira, store,
CLI and the directly affected Gmail/backup integration packages. No test source
files were created; the repository had no existing Go test files. The following
were manual CLI/persistence walkthroughs in a separate private application home:

Formatting and Git whitespace checks passed. Local links across the eight
added/updated entry, contract, setup and validation documents resolved.

| Scenario | Observed result |
|---|---|
| Status and disconnected collect | Status reports a ready saved reader and `not_configured` live reader; collect without a file exits 1 and creates no application directory |
| Saved-input inspection | Version, provider format, policy, timestamps and source digest accepted without account setup |
| First Cloud-shaped batch | One page, three rows/issues, partial with page budget reached |
| Resume | Same acquisition ID; two pages and eight rows produce three unique issues; retrieval ends but known gaps keep the snapshot partial |
| Duplicate and conflict | One duplicate merges evidence; one conflict retains first facts and both source references; neither adds a second issue |
| Scope and assignment | Outside project, unresolved selection and outside-selection dispositions remain explicit; another person's sprint issue has assignment false and sprint membership true |
| Missing values | Null/unreturned estimates remain null; a returned zero remains zero; an uncollected history is `not_collected` |
| Temporal/history limits | Date-only due date unchanged; late source update marked; partial returned comments stay partial; no new task inferred from a later comment |
| Empty input | Explicit final empty page becomes collected with zero issues and complete snapshot |
| Saved REST-v2 shape | Identity-field selection works; string-encoded sprint is unsupported; null comment total is partial, not complete |
| Explicit remapping | New acquisition links to the original, source hashes match, example points change 3 to 5, old evidence remains unchanged |
| Deterministic replay | Same mapping produces identical normalized issue facts after excluding acquisition-specific evidence paths |
| Raw source export | Exported response bytes exactly equal the preserved response file |
| Row budget | Two-row cap stops explicitly; resume exits 1 instead of silently increasing the cap |
| Edited external source | Adding a missing page to the external input does not replace the preserved bundle; resume fails with `reader_failed` |
| Cursor loop | Repeated continuation stops with `repeated_cursor`, retaining eight processed rows |
| Unknown end | Missing end/continuation metadata remains partial, distinct from completed empty retrieval |
| Invalid configuration | Empty timezone and inconsistent REST-v2 offset/cursor rejected |
| Invalid facts and repeated context | Non-string summary/status, nonnumeric points and non-timestamp sprint dates marked invalid; points remain null; duplicate comment IDs reduce history to a partial first observation |
| Text cap | Long Korean/emoji text clips to 131,070 valid UTF-8 bytes under the 131,072-byte cap, with truncated/partial status |
| Mixed index | One manually seeded synthetic legacy pre-snapshot Gmail record appears only in Gmail listing; ten Jira records appear only in Jira listing; Jira show rejects the Gmail record |
| Backup and restore | Backup and manifest verification pass; separate-root restore yields an identical normalized Jira snapshot |
| Corruption | Changing a restored raw response makes Jira show exit 1 with an evidence integrity mismatch; the original synthetic bytes were restored afterward |
| Private state | All 100 acquisition files checked were 0600, and their directories 0700 |
| Side effects | Isolated DB has zero jobs, zero source coverage and zero enabled schedules; schema stays version 4 |

The mixed Gmail record was manually seeded to exercise the legacy parser before
its snapshot exists. This is not a new Gmail API collection or pagination check.
One local walkthrough helper initially looked for a `.sqlite` filename instead
of the existing `state/chunsu.db`; it stopped before DB seeding. The remaining
mixed-storage/backup checks were continued with the observed database path.

## Not run and remaining limits

- No real Jira service requests, token handling, identity verification, field
  discovery, provider retry/rate-limit behavior or Cloud/Data Center live test.
- No new Gmail collection, AI executor run, mail report generation or user
  usefulness evaluation. Existing private mail data and active controls were
  not changed.
- No process-kill, power-loss, disk-full or prolonged-operation experiment. The
  raw-before-normalization checkpoint path is implemented; the manual resume
  evidence above is page-budget continuation, not proof of crash durability.
- No separately fetched comments, changelog, attachments or related-issue
  context. Unsupported source shapes remain preserved and explicitly limited.
- No full-project validation, new automated test suite, distribution build,
  remote configuration, push or publication for this work unit.
- Immutable source, raw and checkpoint copies use additional disk space; the
  pinned evidence byte limits are not a lifetime retention quota.

The next external-interface decision is the Jira deployment/authentication and
scope needed by P6-02b. The next reporting task is P6-04 and the shared execution
dispatch boundary. Neither requires changing this collection model into mail.
