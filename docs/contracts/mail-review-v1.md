# Mail Review v1 — Meaning and Initial Cases

Date: 2026-09-06. Status: public requirements with explicit personal-policy unknowns. The Gmail pilot account is connected, and a scoped real candidate report has been generated. The [pilot reporting baseline](mail-pilot-reporting.md) supplies the current trial rules; human semantic acceptance remains pending.

## Meaning

Use the user's three categories: new work requests/tool-delivered work, interview scheduling, and other irregular requests. Source channels such as Jira are separate from category and do not automatically mean new work. Permit additional category tags where the content overlaps, retaining one stable business-item identity.

Every reported item links to its source messages and explains classification, importance, requested action, and uncertainty. Messages, threads, and business items are different entities. A message may support several items; several messages may support one item. Every target source is included, merged, reference-only, excluded, or unresolved with a reason.

Report explicit deadlines and direct schedule impact. Distinguish proposed, confirmed, changed, cancelled, and unknown schedule states. Without separately authorized calendar data, do not assert an actual calendar conflict. Spam exclusion is a report disposition, never a Gmail mutation. Low importance is not spam.

Previous reports are interpretations, not ground truth. Connect follow-ups through sources, preserve user corrections, and never treat missing completion evidence as proof of unfinished work. Snapshot as-of time and timezone are required; future messages are unavailable to earlier cases.

The current authorized trial uses changes within its collected batch. The code can support both changes and changes-plus-open modes; this trial is not a complete account-wide task tracker or a permanent personal-mode decision. User role and detailed importance preferences remain editable public workgroup assets, with unknowns visible in output.

## Case catalog

| Case | Purpose | Required distinction |
|---|---|---|
| New request | Explicit work request with a deadline | Request versus ordinary reference; evidence for action and date |
| Tool notification | Update sent through Jira/Confluence | Source channel versus new business work |
| Interview sequence | Proposal, confirmation/change, and cancellation at different snapshots | Current state and effective time; no future-data leakage |
| Combined request | Several actions in one mail and follow-ups in another | Many-to-many source mapping without duplicate work |
| Excluded content | Clear synthetic spam versus normal low-priority reference | Auditable exclusion without hiding a normal request |
| Missing context | Unavailable thread or unsupported attachment | Known partial input versus full completion |
| No new mail | A later snapshot with no new target messages | Chosen history mode and unknown completion status |
| Adversarial source | Mail text asks for secrets, rule changes, or unrelated tools | Source instructions do not override host permissions |

Initial executable examples will be explicitly synthetic, without real personal or business data. Expected facts, prohibited claims, permissible wording, source grounds, and reviewer status are kept outside executor packages. Human judgments are needed before these become a personal golden set. This catalog covers the mail requirements M1–M9 in the readiness review; it is not executed validation.
