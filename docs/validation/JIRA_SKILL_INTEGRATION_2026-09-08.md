# Jira and explicit Skill integration — 2026-09-08

## Scope and status

The user approved the Jira integration proposal and requested Skill-managed
instructions explicitly injected into the executor, then authorized development.
The approved grouped workflow is implemented through the existing Go/SQLite
queue, attempts, gateway, artifacts, evaluation, proposal and schedule seams.
No parallel framework or database migration was introduced.

This checkpoint covers P6-01/02/03/04/06/07 only for own-assigned non-Done
issues: overdue, due today through 14 days inclusive, and undated TODO.
P6-05 capacity calculations, sprint/history reporting, human usefulness,
candidate adoption and enabled recurring operation remain unestablished.

## Delivered behavior

- `workgroups/mail-review/SKILL.md` and `workgroups/jira-report/SKILL.md` are
  canonical instructions. A versioned bundle selects one Skill; the package
  preserves its exact Markdown, identity and SHA-256 and injects it directly
  into the executor instructions. Mismatches fail before model execution.
  Legacy mail bundle bytes/digests remain valid.
- `evaluation-skills/golden-evaluation/SKILL.md` is independently pinned in
  evaluation records. It and golden expectations are excluded from report
  execution packages. `pin-evaluator` preserves instructions; an independent
  evaluator still produces the imported judgment. There is no hidden automatic
  evaluator invocation.
- The live Cloud connector uses a user-owned Keychain reference and personal
  API-token Basic authentication. It verifies identity, board/project,
  configured TODO status and date fields, then uses fixed GET routes and
  bounded metadata/description collection. Credentials never enter the model
  package. Comments, attachments, changelog and writes remain excluded.
- Input pins collection/report policies, source identity and as-of date.
  Resume and queue preserve these bindings. The executor can only use the
  attempt's immutable `jira_issue_get`; all selected issues must be looked up.
- Live execution requires a completed nonempty synthetic proof using the exact
  selected Skill bundle and executor. A candidate needs its own proof and can
  be compared without changing the active pointer. Changing proof/policy clears
  the live approval flag until explicitly set again.
- Interval scheduling dispatches the provider and starts disabled. Calendar
  weekday scheduling is not implemented. Restore clears live approval, proof,
  connection activation and secret references; it does not delete Keychain items.

See [the usage guide](../setup/JIRA_REPORTING.md) for actual CLI commands.

## Observed execution and independent evaluation

Private artifacts are stored outside Git in the host application validation
area (`validation/js09`). IDs identify evidence without publishing company
issue bodies, account data, tokens or runtime databases.

| Run | Job / selected attempt | Observed result |
|---|---|---|
| Synthetic Jira baseline | `2AOAKGIYUGS6DFHCTS4JQDYMKX` / `3GLFZVKFHLPXR5UBDKFP2NMPAA` | Completed; 4/4 source lookups; independent 6/6 pass |
| Legacy mail regression | `4TNQE5LU6X4E2N3ND3MT2NBKFX` / `ZH7DWMRQFGGO4O7ZYYC6XNNRVI` | Completed using preserved v1 mail bundle and new explicit Skill package; independent M1–M9 all pass |
| Live Jira baseline | `MWWZGQCMIWFDF4RK6RNKAJ5SWC` / `MCIRS37KOQEMITPEWVW5XKM45J` | Partial report; 26/26 source lookups; independent 6/6 pass |
| Synthetic Skill candidate | `3FDMC7UAQ255H6ACT6CKNKHA7H` / `ZMD557NS4YP6E3EZVYI4U7LVYV` | Completed; 4/4 source lookups; independent 6/6 pass |
| Resumed live Skill candidate | `TZFHS7AIX63JI2CINYP7UIYYQ6` / `GHPLKZCY6ADBO2LFKHGHJ23NKP` | Partial; 26/26 source lookups; independent 6/6 pass; request-context mismatch excludes controlled comparison |
| Controlled live Skill candidate | `UNIHO5CFHI6676VKX2OKQJZEW4` / `YQ6BTYDTL3W73TC3E3ETGB7MZQ` | Partial; 26/26 source lookups; independent 6/6 pass; comparable with baseline |

The live acquisition `NSVLQYXVEXD52EMJ4JS5Q3URGG` completed in one page:
7 overdue, 3 due within the inclusive window, and 16 undated TODO issues.
Independent raw-to-normalized inspection matched all 26 issues' dates and
statuses. Eight source descriptions are empty, so reports honestly retain
limited-content/partial status; collection and tool access did succeed.
Passing semantic checks does not convert partial content into complete content.

Cases and rubric were frozen before the respective baseline model runs. The
same six criteria cover scope, grouping, dates, source grounding, uncertainty,
and source-instruction boundaries. The synthetic input includes a malicious
instruction in source text and was handled as data.

| Evaluation asset | Immutable ID |
|---|---|
| Synthetic case | `XTXICEWSEIAEU43UIE5S6OR64Z` |
| Live case | `A4VIOVSEUGYLH6PLELXEY6T2AP` |
| Shared rubric | `LVC7EJOLHYEOS6BAVCHSLVLARH` |
| Independent evaluator Skill | `G3GYN4KVXDXACCPMTEMVHAXTHK` |
| Synthetic baseline evaluation | `U6M2MJ732OPXKE4AEBY33TVLII` |
| Live baseline evaluation | `OCJNBT6GEISEL7ZWSZNJYH2DCQ` |
| Synthetic candidate evaluation | `EF2I6S7UNRIIIKC35DLFFZTWYG` |
| Resumed live candidate evaluation | `BXRFZC3RZTAVOPYGJBF6DIZV66` |
| Controlled live candidate evaluation | `ET57TKVVP2YBNHWA2RRSOZOB7R` |
| Controlled live comparison | `NI7COLC5OJMJ5Z2ZZSDCFYUUBK` |
| Mail regression evaluation | `IQBOVWESTGT45VWTTBGRXUKDWT` |

The mail regression used the case preserved before its run and the existing
unchanged mail rubric. Its records were imported afterward: case
`VPG2LWDFNBDC2EK7RA23O5UZLJ`, rubric `WIZPWAO3V5BZFMWJTKXW2J7YUA`, and separate
mail evaluator Skill `6VASFJBF6TZAWRU65QQVDKDVBZ`. All nine judgments include
source references and preserve the distinction between source facts, unknown
completion, human usefulness and adoption.

The candidate proposal `MBSTCPO45TM7TFWQAFKIFECXWK` tests plain-language gaps
and the distinction between date expiry and actual delivery delay. Its inputs
were already inspected; it is a tuning comparison, not held-out confirmation
or proof of general improvement. It remains inactive pending a human decision.
The controlled comparison is `comparable: true` with no comparison limitations:
input, request context, model/runtime, limits, case, rubric and evaluator match.
Only the per-Skill synthetic proof job ID is excluded from executor-config
comparison. Both results pass; the candidate removes the successful technical
retrieval statement from gaps. No general quality increase or human usefulness
is inferred. The original active Jira bundle and its proof selection were
restored/verified after candidate experiments.

## Failures retained and corrected

The first synthetic attempt is preserved as failed. All four tools were
called, but the new host Jira evidence decoder rejected additional valid
fields, and the MCP output schema advertised a byte array for an object.
Both integration defects were corrected; the subsequent attempt used the typed
object schema and completed. These are not a third model failure to look up
required sources from the earlier evaluation loop.

Connection validation exposed two distinct defects: personal API tokens need
Basic authentication, and the configured TODO status need not be in the first
board column. Both were corrected before the successful bounded collection.
An overly long validation directory exceeded the macOS socket path limit;
the idle private fixture was copied to the shorter `js09` directory without
discarding the original evidence.

One candidate live invocation was blocked before model launch because changing
the synthetic proof intentionally cleared live approval. The existing user
authorization was applied to the new proof and the blocked attempt retained.
Its recovery note changed the pinned request context, so comparison
`FUAENUWIEKV6AQON4VIVCCZ55Y` correctly reports `comparable: false` despite the
matching source input and passing semantic judgments. A fresh candidate job
uses the original request context for the controlled comparison.

Final review also corrected legacy-mail publication compatibility, cumulative
live resume bounds, retry-wait persistence, exact active-Skill schedule proof
checking, and empty connection listing in a fresh home. The Markdown renderer
now uses meaningful Jira links, grouped/TODO counts, date highlights, expired
days and a compact undated table. It derives facts from the existing contract;
it does not invent code-work candidates or capacity judgments.

## Validation limits

Existing Go tests and real CLI/model checks are used; no new test files were
created. Skill format checks cover all three Skills. Negative checks rejected
a mismatched candidate Skill before queue/model execution and rejected a mail
job as Jira live proof, preserving the active controls.

`go test ./...` passed after the resume/schedule/publication corrections; most
packages have no test files, so their result is compilation rather than case
coverage. Focused runner/Jira/CLI checks passed after the rendering change.
`git diff --check` passed. An actual `publish` call recovered/verified the legacy
mail report without invoking the model again. Fresh-home `jira connections`
returned an empty array.

A real backup was verified and restored to the separate private `validation/jsr`
root. Checks confirmed a paused queue, no schedules, disabled Jira connection,
cleared Keychain reference, cleared live approval/policy/proof, and an unchanged
original connection file. No credential value was inspected for this check.
In that isolated restored fixture, removing only the presentation record and
setting the publication-recovery checkpoint demonstrated Jira `publish` from
preserved output without another model run. The final renderer produced all
26 Jira links, 17 TODOs split into 1 dated and 16 undated, the correct calendar
day differences, and retained partial status. Original runtime evidence was
unchanged. Final focused Jira/CLI/runner/feedback tests and build passed after
the named window-constant and TODO subtotal cleanup.

No second executor adapter, adversarial live-provider fault campaign, complete
crash matrix, weekday scheduling, Jira writes or human usefulness acceptance
has been claimed. The validation connection is isolated from the normal data
root; no production schedule was created or enabled.
