# Jira ingestion without an account

The approved collection layer is available with saved responses. Real Jira
login, HTTP access and AI Jira reporting remain deferred. No token, GCP project,
database service or new runtime dependency is needed for this walkthrough.

Build the Go CLI, choose a dedicated private directory with `--home` or
`CHUNSU_HOME`, and initialize it. The commands below use the selected home:

```sh
go build -o bin/chunsu ./cmd/chunsu
bin/chunsu setup
bin/chunsu jira status
bin/chunsu jira inspect examples/jira/saved-cloud.json
bin/chunsu jira collect examples/jira/saved-cloud.json
```

The example is explicitly synthetic and has a one-page invocation budget. The
first result is `partial`, with three issues and `page_budget_reached`. Copy the
returned `acquisition_id` into the following commands:

```sh
bin/chunsu jira show ACQUISITION_ID
bin/chunsu jira show ACQUISITION_ID --snapshot
bin/chunsu jira source ACQUISITION_ID 0
bin/chunsu jira resume ACQUISITION_ID
bin/chunsu jira acquisitions
```

`show` displays policy, progress and preserved file references. `--snapshot`
displays normalized facts and limitations. `source` prints exact response bytes
for a zero-based page index. After resume, eight rows have been processed into
three unique issues. Retrieval is complete, but the snapshot remains partial:
the example includes conflicting observations, incomplete history, an outside
project and unknown selection evidence. Another person's sprint ticket stays
distinct from the subject's assigned work.

Reprocess the original preserved source using another example mapping:

```sh
bin/chunsu jira renormalize ACQUISITION_ID --mapping examples/jira/alternate-mapping.json
```

This creates a **new** acquisition and `reprocess_of` link. The first issue's
example story points change from 3 to 5 because the mapping selects a different
field. The old result and raw source stay intact. Resume the new ID to process
its next batch. Editing the external input file does not change an existing
acquisition's saved source.

Other examples are `empty-cloud.json`, an explicitly complete empty result, and
`saved-rest-v2.json`, a saved older response shape with unsupported sprint data
and incomplete comment metadata. The latter does not prove live Data Center
support. Example users, projects and custom field IDs are fictional.

## When collection stops

| Result or diagnostic | Next action |
|---|---|
| `not_configured` | Supply a saved bundle now; select the live Jira deployment/authentication in a later step |
| `page_budget_reached` | Resume the same ID for another manually bounded batch |
| `issue_budget_exhausted` | Review the raw page; use an explicit new bundle/policy if a larger scope is needed |
| `continuation_unavailable` | Obtain a source bundle with explicit end/continuation evidence; do not interpret this as an empty completed result |
| `reader_failed` with unavailable saved continuation | Start a new acquisition containing the additional pages; the old source remains pinned |
| `repeated_cursor` | Correct the source continuation and begin a new acquisition; do not retry the loop indefinitely |
| `evidence_budget_exhausted` or byte-limit refusal | Reduce the next acquisition's scope or explicitly review host limits |
| `normalization_failed` | Preserve the ID and raw source, inspect the mapping/format and correct the adapter; do not mark the material reported |
| Integrity or binding mismatch | Preserve the files and recover from a verified backup; do not change stored hashes to hide the mismatch |
| `collected` with snapshot `partial` | Inspect gaps and field/history availability before making any report judgment |

`collect` without a source file returns `not_configured` with a failing exit
status before loading application settings or contacting any account. Expected
partial results use exit status zero and explicit status fields. Reader and
integrity errors return a failing exit status.

The existing `backup`, `verify-backup` and separate-directory `restore` commands
include Jira acquisition files. Jira input, raw responses and snapshots use
private permissions and manual retention. The current run-retention command
does not remove these acquisition copies. Keep real source bundles outside Git.

## Later connection work

Use the [connection preparation checklist](JIRA_CONNECTION_PREPARATION.md) for
deployment, authentication, scope and the implementation/validation sequence.
The [September 7 readiness check](../implementation/STATUS_2026-09-07.md)
records the current source inspection and rerun saved-input walkthrough.

The next dependent unit is selecting Cloud or the company's deployed Data
Center version, base service identity, authentication mode and a concrete
project/user/sprint scope. Actual custom field IDs must be discovered or
supplied. Credentials belong to a host secret reference, outside the source
bundle, normalization and executor package.

Then implement the selected read-only HTTP reader and its connection lifecycle
against this boundary. Jira reporting separately needs its guide/result schema,
scoped source tools and workgroup dispatch through the existing executor. Those
are not activated by collecting data.

See the [evidence contract](../contracts/jira-evidence.md) for exact meanings and
the [validation record](../validation/PHASE_06_JIRA_INGESTION.md) for checked and
unchecked behavior.
