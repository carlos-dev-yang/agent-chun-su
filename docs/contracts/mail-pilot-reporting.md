# Mail pilot reporting baseline

Date: 2026-09-06. Status: candidate for the user-requested synthetic and live trial. Human feedback can revise this baseline; testing does not silently adopt it as the active workgroup.

## Scope and categories

Use the connected account's seven-day inbox policy, a maximum of 20 listed messages, no related-thread expansion, Asia/Seoul and manual retention. Gmail list requests explicitly exclude spam/trash; mail outside the collected batch is neither reviewed nor counted as an AI exclusion. No mailbox or calendar changes are permitted.

Keep the existing v1 categories: new work/project requests, interviews and their schedules, and other irregular/direct operational requests. Show a source-supported project/topic in the item title. Categories describe work, while Gmail/Jira/etc. describe its channel. Multiple messages may support one item, and a message may support separate requests. The first category is the primary presentation group.

Operational alerts, maintenance failures and permission-review notices belong to the operational category unless they also contain an explicit new assignment. Suggested investigation does not create an assigned project task. Opaque IDs are not evidence for a person's or project's display name. This clarification comes from reviewing the first actual candidate report and is retained as a separate candidate revision.

Exclude spam, advertising, sales promotions and promotional newsletters from the main summary. Keep an auditable source disposition with a reason code and evidence-based explanation. Do not classify all automated messages as advertising: payment failures, security changes, service incidents and direct requests may require action. Ordinary completed receipts or informational updates can be reference-only. Ambiguous intent stays visible as uncertainty rather than disappearing into an exclusion.

Importance follows concrete action, deadline and operational impact. Marketing urgency does not create a deadline or high-priority task. Missing completion evidence does not establish an unfinished task. The trial uses changes within the collected batch, not a complete account-wide open-work tracker.

## User-facing artifacts

1. A Korean Markdown report groups actionable business items by primary category, states the next action and schedule/importance evidence, and links each item to its exact supporting sources. Status labels and displayed timestamps use Korean and the pinned report timezone; raw schema values stay unchanged. The executive summary gives exclusion counts while leaving excluded senders and marketing topics in the source document.
2. A separate local Markdown source document assigns deterministic labels within the immutable snapshot. It contains original source IDs, sender, subject, received time, thread ID, source-to-item mappings, exclusion/reference reasons, normalized body text and known attachment/body limitations. Source text is rendered inertly; its URLs and embedded instructions are not executed.
3. The preserved input snapshot, raw AI JSON, pinned guide/schema manifest, lookup journal and host validation remain the machine-readable evidence. The local text view is not a complete original EML or an attachment archive. Existing Gmail originals are not moved, relabeled or marked read.

The report excludes marketing details from its main items but links to the source document for reviewing exclusions. Collection gaps, unreadable bodies and further available pages remain explicit. An empty actionable report is a valid outcome if the evidence supports it.

## Indirect executor handoff

The human-authorized host stores a candidate workgroup version. For each attempt it pins the guide, existing v1 JSON schema, source index, as-of/timezone and budgets into an immutable package. The executor gets that package and the single `mail_source_get` tool. It chooses how to classify, merge and summarize the permitted sources; the host does not supply prewritten summary answers.

Gmail credentials, connection management, active controls and private evaluation expectations remain on the host. The host validates source coverage and references, then generates source links and Markdown presentation from the preserved input. Classification quality is reviewed separately from these structural checks.

## Trial evidence required

- A synthetic real-executor run checks available capabilities, an out-of-scope source denial and resistance to source-injected instructions. Non-production markers are used for filesystem/control checks; no real secret is used as probe material.
- A policy fixture includes direct work, changed interview scheduling, a transactional action, pure promotion, an ordinary receipt and an ambiguous message. Expectations stay outside the executor's package.
- After the configured access boundary passes, collect only the bound live batch and run the candidate through the same controller/package/gateway path. The user's request authorizes this bounded live disclosure after verification.
- Preserve partial and failed attempts. Review source coverage, actual exclusions, grounded next actions and the usability of the source links. Live usefulness and adoption remain human judgments.

## Observed trial and remaining decisions

The [actual executor check](../validation/EXECUTOR_BOUNDARY_2026-09-06.md) and
[live pilot record](../validation/PHASE_04_GMAIL.md#current-bounded-live-report)
now supply concrete evidence. One 20-message batch was collected, then analyzed
twice using separate pinned candidates. The revised report has five items and
four exclusions, with links to every included source. Additional mailbox pages
remain and were not collected.

The revised operational classification and Korean presentation worked in this
sample. A notification with no underlying message content changed from
reference-only to an uncertain item; whether such notices belong in the main
report needs user feedback. Item order still did not consistently follow the
guide's importance order. These are recorded quality findings, not reasons to
declare a structurally valid report invalid or to run an unbounded retry loop.
The active rules were not promoted, and neither candidate advanced production
coverage. The stored comparison has no independent semantic judgments and
does not establish improved personal usefulness.
