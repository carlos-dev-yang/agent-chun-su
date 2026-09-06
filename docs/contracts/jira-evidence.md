# Jira saved-source and evidence contract

Version: 1. Normalizer: `jira-evidence-v1`. Date: 2026-09-06.

This contract implements the approved [ingestion boundary](../implementation/JIRA_INGESTION_PREPARATION.md).
It describes local collection and evidence, without authorizing a Jira account,
claiming a historical atomic view, or defining an AI reporting rule.

## Source envelope and replaceable reader

`SavedInput` contains `version`, `provider: jira`, `format`, `synthetic`,
`policy` and `pages`. The supported saved response shapes are `cloud-v3` and
`rest-v2`. These names do not assert live Cloud or Data Center compatibility.
Every page has an opaque request `cursor`, `captured_at` and a raw JSON `body`.
The initial cursor is the empty string. Cursors are unique within the bundle.

The internal `Reader.ReadPage(context, Request)` accepts the pinned policy and
cursor, returning response bytes, capture time, continuation and an explicit
end-of-source indicator. No credentials enter normalization. The saved reader
owns pagination interpretation; a future HTTP reader owns authentication,
routes and provider error handling. `DisconnectedReader` returns
`ErrNotConfigured` without account, network or Keychain access.

Cloud-shaped pages use `nextPageToken` and `isLast`; a missing continuation
without explicit `isLast: true` is incomplete. REST-v2-shaped pages require
consistent `startAt`, `total` and issue count; each saved cursor must match the
offset. A missing saved continuation is an error, even if the external input
file was subsequently edited. New pages require a new preserved acquisition.

The original bundle is copied exactly. Each fetched page body is also preserved
as the exact JSON value bytes supplied to normalization, including its internal
whitespace. This is a saved API response archive, not proof of an HTTP exchange:
capture timestamps and the synthetic flag are supplied by the input author.

## Pinned policy and mapping

| Field | Meaning |
|---|---|
| `connection_id` | Stable local namespace, 16–64 ASCII letters/digits/underscore/hyphen; not an active credential binding |
| `project_keys` | Explicit allowed projects; empty or duplicate IDs are rejected |
| `subject.id` | Explicit provider-native user identity; saved data does not verify a login |
| `selection` | `assigned` or `assigned_or_sprint` |
| `sprint_ids` | Explicit sprint IDs; no inference of the current sprint |
| `timezone` | Explicit IANA zone; timestamps normalize to UTC, due dates stay calendar dates |
| `max_issues` | Maximum source rows processed across the acquisition, including duplicate/excluded rows |
| `max_pages` | Maximum pages processed in one collect/resume invocation; another manual resume permits another batch |
| `max_context_entries` | Maximum entries retained per issue per returned comment/changelog collection |
| `mapping.version` | Mapping contract version, independent of source format |
| `mapping.identity_field` | Explicit `accountId`, `key`, or `name`; no fallback guessing |
| `mapping.sprint_field` | Actual custom field ID or empty for unmapped |
| `mapping.story_points_field` | Actual custom field ID or empty for unmapped; must differ from sprint field |

The custom field IDs in [examples](../../examples/jira/) are fictional.
`assigned_or_sprint` requires a sprint mapping and sprint IDs. Missing selection
evidence produces an unresolved disposition, not a guessed exclusion. Another
person's selected-sprint issue can be included with `assigned_to_subject: false`.
No issue count or sprint membership is interpreted as personal workload.

Host response, text and evidence byte limits are pinned in `byte_budgets`.
They bound a single raw response, each normalized text field, and separately
the source bundle, aggregate fetched response bytes, a normalized snapshot and
each checkpoint. These are not a lifetime disk quota: immutable checkpoints
and reprocessing copies consume additional space. Retention is manual. A lower
current host limit blocks reading/resume of a larger pinned acquisition rather
than silently relaxing the limit.

## Normalized evidence

`Snapshot` binds `acquisition_id`, connection ID, policy digest, provider format,
synthetic flag, normalizer version and timezone. The checkpoint contains the
full pinned policy; the snapshot contains its digest. `captured_from` and
`captured_through` describe the interval of processed page observations.

Issue IDs are `connection_id:provider_issue_id`. A renamed issue key does not
change identity. Every included issue and row disposition references a response
path, SHA-256 digest, byte count, capture timestamp and `/issues/N` JSON pointer.
Use those references to inspect exact source evidence.

| Normalized group | Interpretation |
|---|---|
| Current facts | Summary, description, source status ID/name/category key, assignee, resolution and source timestamps |
| Sprint membership | IDs, names, state, available board ID and dates; assignment and sprint flags are separate |
| Estimates | Original/remaining/spent **seconds** from `timetracking`, and explicitly mapped story points; values are nullable |
| Due date | Original `YYYY-MM-DD` calendar date; no invented midnight or UTC conversion |
| Relationships | Source link type and inward/outward direction, namespaced related issue ID/key, `context: not_collected` |
| Comments/changelog | Returned entry IDs, source author identity/time, bounded body/change facts and explicit history coverage |
| Availability | `available`, `not_returned`, `empty`, `not_mapped`, `unsupported`, `invalid`, `truncated`, or applicable history coverage |
| Limitations | Field conversion failures, truncation, or a source update timestamp later than its page capture |

Plain text is retained; supported ADF nodes are reduced to bounded text. Unknown
rich-text nodes mark partial conversion, and clipping preserves valid UTF-8.
Links are inert text. Attachments, links and related-issue content are never
fetched. Legacy string-encoded sprint objects remain `unsupported`; their raw
data is retained. No status-to-risk mapping or points-to-hours conversion is
performed. Null and absent estimates remain null, distinct from a returned zero.

History is `complete` only when valid returned entries and pagination totals
establish full coverage of that embedded collection within the configured cap.
An absent collection is `not_collected`; null totals or truncated entries are
`partial`. These are the histories provided by a saved response, not separately
fetched histories. A later comment does not create a post-completion task.

Each processed row receives a disposition: `included`, `duplicate`,
`conflicting_duplicate`, `outside_scope`, `outside_selection`, or `unresolved`.
Identical normalized observations merge provenance under one issue. Conflicting
observations retain the first facts and all source references, set
`conflicting_versions: true`, and add a gap. They are not silently replaced by
an assumed latest truth. Budget-truncated rows remain in the raw page and are
represented by an explicit `issue_budget_exhausted` gap.

## Persistence and completion

All files are under `state/acquisitions/ACQUISITION_ID/`, using the existing
private file store and SQLite acquisition index. No schema migration is needed.
Jira creates no entry under Gmail's `state/connections` directory. The provider
discriminator lets acquisition lists recognize Jira and legacy Gmail records.

The original bundle is persisted before the initial checkpoint. A fetched raw
page is persisted and checkpointed with `processed: false` before normalization.
After conversion, a new snapshot is written and the checkpoint advances its
row count and cursor. Resume can reuse a pending raw page. Files are immutable;
SQLite publishes the current checkpoint reference after the file write.
Unpublished files left by an interrupted write may remain for manual retention.

| State | Meaning |
|---|---|
| `collecting` | Last checkpoint still has work; it may be left by interruption |
| `partial` | Invocation stopped at a page/issue cap or lacks continuation |
| `failed` | Reader, evidence, progress or normalization error; `stop_reason` identifies the class |
| `collected` | Selected source traversal reached its explicit end within the row budget |
| `retrieval_complete` | Separate fact that traversal ended; it does not mean a report exists |
| Snapshot `complete` | Traversal ended with no known conversion/selection gaps; field and history availability still apply |
| Snapshot `partial` | Retrieval is unfinished or known evidence gaps remain |

`show`, `resume`, `source` and `renormalize` verify referenced bytes, hashes,
policy binding, original saved page metadata and normalized provenance before
using a record. Hashes detect corruption; they do not authenticate a provider or
an input author's claims. No claim of one historical state follows from the
capture interval.

`renormalize --mapping` starts a new acquisition from the preserved source
bundle with an explicit mapping, recording `reprocess_of`. Previous records
remain immutable. The normal page cap still applies; the new acquisition may
need its own resume. Scope changes require an explicit new source/policy bundle.

No job, executor package, report coverage, schedule, account grant or secret is
created by these commands. Jira guide/schema, scoped executor tools and shared
workgroup dispatch are later work. See the [terminal guide](../setup/JIRA_INGESTION.md)
and [actual validation](../validation/PHASE_06_JIRA_INGESTION.md).
