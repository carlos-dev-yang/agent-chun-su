# Jira automatic collection and report implementation proposal

Date: 2026-09-07.

Update: the user approved the grouped direction and requested one report first,
explicitly requiring undated TODO and overdue issues to appear. On 2026-09-08
the user accepted this integration proposal and required the work instructions
to be managed as Skills and explicitly injected into the executor. The first manual
[report checkpoint](../validation/PHASE_06_JIRA_FIRST_REPORT.md) uses
`own_assigned_14d_grouped_v1` and bounded descriptions. Actual schedule timing is
still unselected. Alternatives below are retained for context; the selected
grouped scope and bounded descriptions do not require reselection. Application
integration and explicit Skill delivery remain work to implement.

The user's subsequent concise Markdown layout is recorded in the
[report presentation guide](JIRA_REPORT_PRESENTATION.md) and applied to the
manual report. It does not mark the runtime report contract as implemented.

Status: integration implementation authorized, with explicit Skill injection
required. The user has authorized the Jira Cloud read-only collection and report
feature, including the already verified API access. This approval does not
establish implementation completion or enable a recurring schedule. This feature
does not include comments, transitions,
field changes, reassignment, ticket implementation, or autonomous source-code
work derived from Jira issues.

## Intended first result

One Jira Cloud acquisition can be queued as a `jira-report` job and travel
through the existing job, attempt, evidence, validation, report, evaluation and
proposal lifecycle. The implementation adds provider-specific adapters at the
existing seams; it does not add a second runner, queue, evidence store or
evaluation framework.

The first useful run is manual. Add Jira provider dispatch to the existing
elapsed-interval scheduler as part of this feature, leaving actual schedules
disabled until their timing is chosen. An optional weekday 09:00 Asia/Seoul
schedule requires a distinct calendar recurrence change. The initial manual
and elapsed-interval implementation needs no database migration.

## Selected behavior and deferred schedule

### Report selection

Selected `own_assigned_14d_grouped_v1`:

- include issues assigned to the authenticated user whose status category is
  not Done and whose due date is before today;
- include issues assigned to the authenticated user whose status category is
  not Done and whose due date is from today through today plus 14 calendar
  days, inclusive;
- include undated issues assigned to the authenticated user only when their
  status ID is the reviewed TODO status ID;
- show the three groups separately. Do not add their counts together as effort
  or capacity, and do not label an overdue date as a confirmed delivery delay.

Alternative `own_todo_due_14d_v1` includes only issues assigned to the user,
with the reviewed TODO status ID, and due from today through today plus 14 days.
It excludes overdue and undated issues.

The recommended date interpretation is Jira date-only values in `Asia/Seoul`.
`due_date < as_of_date` is overdue and the upcoming window is
`as_of_date <= due_date <= as_of_date + 14 days`. The start date is displayed
as source evidence; it does not exclude an issue unless the user later adopts
that rule. The first report uses `duedate` as the deadline and the reviewed
start-date field as the start date.

### Schedule

Initial mode `manual_validation`: collect and run one report only when requested.
Alternative `weekdays_0900`: one run at 09:00 Monday through Friday in
`Asia/Seoul`, with existing missed-run and intervention behavior preserved.

The current schedule is interval-based and Gmail-specific. Weekday wall-clock
recurrence needs a separately reviewed schedule representation and likely a
schema migration. It must not be approximated by a 24-hour interval. Defer that
change until the manual report is accepted.

### Source content

Selected `metadata_and_description`: collect the bounded issue fields below
and a bounded description so the report can explain work meaningfully. Exclude
comments, attachments and changelog from the first report. Bounded descriptions
were included in the approved manual report and the 2026-09-08 real-source
capture. The implemented connection policy and report must retain that scope.

An alternative `metadata_only` avoids description reads but may produce a less
useful report. Neither option permits executor access to Jira APIs directly.

## Connection profile contract

Store a strict JSON profile under the existing private connection area. Values
observed for the current tenant become profile values created through the CLI,
not constants in source code.

| Field | Validation and meaning |
|---|---|
| `version` | Integer `1`. |
| `id` | Generated local connection ID. |
| `provider` | Exact value `jira-cloud`. |
| `site_host` | Exact HTTPS host, no scheme/path/query; current reviewed value is `cjenm.atlassian.net`. |
| `cloud_id` | Canonical UUID; current reviewed value is `c47428a0-0175-44da-baea-b04169020dc9`. |
| `expected_account` | User-reviewed email used only to verify `/rest/api/3/myself`; current value is `carlos-yang@cjenm.com`. |
| `subject_account_id` | Stable account ID returned by verified identity; required before activation. |
| `project_key` | Reviewed Jira key; current value is `SWMPFE`. |
| `board_id` | Positive decimal string; current value is `1704`. |
| `todo_status_id` | Reviewed status ID; current value is `10084`. |
| `todo_status_name` | Observed display name for presentation, currently `Backlog`; ID controls selection and a name change is reported rather than silently rejected. |
| `due_field` | Exact reviewed field key, currently `duedate`. |
| `start_field` | Exact reviewed custom field key, currently `customfield_10015`. |
| `timezone` | Valid non-`Local` IANA zone, currently `Asia/Seoul`. |
| `secret` | `{store:"macos-keychain",service:"chunsu-jira-cjenm",account:"carlos-yang@cjenm.com"}`; contains no token. |
| `policy` | Pinned report/acquisition policy described below. |
| `active` | False until identity, board, project and fields are verified with GET requests. |

Recommend an explicit `jira connect --keychain-service ... --keychain-account
...` import that creates this non-secret profile and verifies it. It reads the
existing Keychain item at request time and never copies the token to project
configuration, artifacts, logs, command arguments or executor packages. Backup
includes the profile but continues to exclude Keychain values.

This binding references a user-owned external Keychain item. Disconnect disables
the local profile and does not delete that external item. Restore must recognize
the profile provider, disable the Jira connection, clear its secret binding and
clear Jira live-executor approval. It must not parse Jira profiles as Gmail.

The HTTP adapter permits GET only to fixed Atlassian origins and reviewed route
templates. It rejects redirects, arbitrary URLs and non-GET methods. Issue
collection uses the observed board-aware enhanced endpoint
`/rest/software/1.0/board/{board_id}/issue` so board scope is retained, with a
typed, host-built JQL predicate and bounded continuation. Identity and field
verification may use `/rest/api/3/myself`, board/configuration and field lookup
GET routes. Provider errors distinguish authentication, authorization, rate
limit, retryable failure, malformed response and partial pagination.

## Acquisition policy contract

Extend the versioned Jira policy without changing SQLite tables. A new contract
version starts a new acquisition chain; old saved acquisitions remain readable
through their version adapter.

| Field | Rule |
|---|---|
| `connection_id`, `project_keys`, `board_ids` | Nonempty unique allowed scope; requests cannot expand it. |
| `subject_account_id` | Must match verified identity; assignment is compared by stable account ID. |
| `selection` | Reviewed enum `own_assigned_14d_grouped_v1` or `own_todo_due_14d_v1`. |
| `status_ids.todo` | Nonempty reviewed ID; display names are evidence only. |
| `date_fields.due`, `date_fields.start` | Explicit field IDs; missing values remain null/unknown. |
| `as_of_date`, `timezone`, `window_days` | Controller-owned values; `window_days` is 14 for these policies. |
| `content_scope` | `metadata_only` or `metadata_and_description`; comments, attachments and changelog false initially. |
| `limits` | Positive issue/page/response/text/evidence limits and retry budget, bounded by host configuration. |

The connector obtains only fields required by the selected content scope:
issue ID/key, project, summary, status ID/name/category, assignee account ID,
due date, configured start date, created/updated/resolution, and optionally
description. Pagination is complete only when the provider marks the final
page. Each raw response is preserved before normalization; retrieval success,
empty results, partial results and normalization failures remain separate.

The normalized snapshot adds the configured start date with availability and
provenance. It does not compute delay, priority, effort or capacity. The
existing acquisition index and private evidence files can hold the new live
records, so manual collection requires no database migration.

## Report result contract

The executor returns JSON validated against a Jira-specific versioned schema:

| Field | Required validation |
|---|---|
| `version` | Integer `1`. |
| `as_of_date`, `timezone` | Exact values pinned in the input snapshot. |
| `scope` | Connection-independent project key, board ID, subject label, selection ID and content scope; no secret or account ID is rendered. |
| `groups.overdue` | Array of item source IDs for the grouped policy; every referenced item has non-Done category and `due_date < as_of_date`. |
| `groups.due_within_14_days` | Array of item source IDs; every referenced item falls in the inclusive window. |
| `groups.undated_todo` | Array of item source IDs for the grouped policy; every referenced item has null due date and the reviewed TODO status ID. |
| `items[]` | `source_id`, key, summary, status ID/name, due date, start date, concise note and cited evidence IDs. Source IDs must belong to the immutable snapshot. |
| `gaps[]` | Missing/denied/truncated fields, incomplete pagination and temporal inconsistency stated explicitly. |
| `assumptions[]` | Report judgments separated from source facts. |
| Host validation result (outside executor JSON) | Existing lifecycle values `completed`, `partial`, `failed`, or a waiting state are assigned by the controller using acquisition and output evidence. The executor cannot declare operational completion. |

Groups must be disjoint, refer only to declared items, and account for all
selected sources; narrow-policy unused groups are empty. Every selected source
must be retrieved through the attempt gateway even when
the report excludes it after inspection. Validation rejects duplicates,
out-of-scope sources, incorrect group membership and missing lookup evidence.
Instructions prohibit unsupported capacity and delay claims; their semantic
correctness remains a separate evaluation rather than a schema guarantee. Markdown
rendering links each item to preserved local source evidence and makes partial
coverage visible.

## Explicit Skill delivery

The current implementation is not yet a Skill-loading implementation. Mail has
a Markdown `guide.md` that the host combines with the pinned request and source
index into `instructions.md`; the executor receives that text through stdin.
The actual mail package contains `instructions.md`, `source-index.json`,
`report.schema.json` and `executor.schema.json`. It has no `SKILL.md`. Host Skill
discovery is disabled by the current adapter. The two files under `docs/skills/`
are host procedures, not automatically registered or injected runtime Skills.
There is no Jira workgroup Skill or Jira runtime package yet.

The user-required implementation must provide the following:

1. Keep each workgroup's canonical `SKILL.md`, its declared supporting documents
   and output schema outside the executor, under host/user ownership.
2. Select and pin the workgroup Skill and schema version before each attempt.
   Preserve Skill identity, exact content hashes and declared dependencies in
   that attempt's evidence; do not resolve a moving latest version mid-run.
3. Deliver the selected Skill explicitly through the executor adapter, together
   with only its needed supporting files. Preserve the isolated package and
   scoped gateway. Do not rely on host-wide discovery or optional model choice
   as evidence that the required Skill was supplied.
4. Reject a missing, mismatched or changed required Skill before execution.
   Verify the prepared package and actual adapter invocation. Identical Skill
   delivery does not guarantee identical model output; the execution gate and
   independent evaluation must still check the result.
5. Give the independent evaluator its own pinned evaluation instructions.
   Keep golden expected answers and private evaluation records out of the
   report executor package.
6. Demonstrate explicit Skill delivery in a real synthetic executor run before
   the corresponding real-data run. Record source lookups, boundary checks and
   output validation, rather than treating a created Markdown file as completion.

This section records the accepted requirement and observed gap. It does not
claim that the Skill assets or loading path have already been implemented.

## Minimal integration work

1. Implement the Cloud profile, Keychain reference reader and bounded GET
   adapter behind the existing Jira reader interface. Generalize the saved-only
   acquisition replay assumptions while preserving raw-page-first checkpoints.
2. Add start-date/status/date-window fields to the versioned Jira model and
   normalizer. Preserve compatibility with existing version-1 saved evidence.
3. Add the `workgroups/jira-report` Skill and schema, and implement explicit
   selected-Skill delivery as specified above. Adapt the mail instruction path
   with compatibility for its preserved bundles. Select active bundles by job
   workgroup rather than the current mail-only path.
4. Extract a small workgroup runtime interface used by package preparation,
   runner and publication recovery: parse input, verify origin, build source
   index/instructions, validate result, render presentation and optionally
   record coverage. Keep lifecycle and artifact code shared.
5. Dispatch the gateway by the pinned manifest workgroup. Add a bounded
   `jira_issue_get` over the immutable snapshot and generalize lookup evidence
   decoding; retain attempt revocation, policy-digest checks and evidence byte
   limits. No live Jira tool is exposed to the executor.
   Bind the package source kind, source-index digest and live/synthetic marker;
   expose exactly the selected workgroup's tool. A Jira-specific disclosure
   guard binds the executor identity and Jira policy digest. Validate the actual
   synthetic Jira tool boundary before the first live Jira executor run; Gmail
   approval does not automatically enable Jira disclosure.
6. Adapt feedback case fingerprints/import, evaluation reruns and proposal
   bundle selection by workgroup. Jira cases and rubrics then use the existing
   immutable records, evaluation links, comparison and human adoption path.
7. Add Jira dispatch to existing interval schedules, retaining durable tick IDs,
   linked acquisition/job repair, no overlap and explicit blocked occurrences.
   Do not enable a live schedule until its timing is selected. If
   `weekdays_0900` is selected, design the calendar recurrence representation
   in a separate operations increment instead of substituting a 24-hour interval.

Schedule provider dispatch can resolve the existing unique connection ID to the
typed connection profile, preserving legacy Gmail interpretation. Do not add a
schedule-table migration merely to duplicate that provider discriminator. Jira
acquisition/job linkage must remain recoverable when a process stops between
job creation and tick update. Each new occurrence obtains a current snapshot;
it must not reuse a prior cursor as though it represented the current date.

The extraction should contain only behavior needed by mail and Jira. Existing
mail contracts remain adapter behavior; Jira semantics must not enter the
generic controller.

## Completion evidence and bounded commits

The subsequent user request authorizes execution-gate and golden-loop testing.
Use focused tests and controlled inputs in coherent batches:

1. Profile and connector increment: controlled HTTP responses demonstrate
   route/method/origin restrictions, identity binding, pagination, retry/auth
   states and absence of token persistence; then one bounded real GET
   acquisition records raw-to-normalized hashes.
2. Shared execution increment: one Jira acquisition reaches a job/attempt,
   bounded gateway evidence, Jira validation and local report. Run focused
   existing Gmail saved/live paths to show the dispatch extraction preserved
   them. Exercise publication recovery once.
3. Shared evaluation increment: import a Jira case/rubric, record an evaluation
   linked to source evidence, rerun it under the Jira workgroup and record a
   proposal for human review without fabricating adoption or granting Jira write access.
4. Record checks actually run, checks not run, partial evidence and remaining
   schedule decision. Commit each validated increment with its P6 task ID and
   stage only its files.

This planning increment changes no runtime code. No build or tests were run.
