# Persistent chat operations goal — 2026-09-13

Status: active. The user explicitly requested short mechanical acknowledgments,
accumulated internal error reports, chat-based management and supported feature
installation, recovery from channel/process failures, supervised operation until
explicitly stopped, and simple local/server installation.

## Direction and completion evidence

| Unit | Work | Completion evidence | Status |
|---|---|---|---|
| CHAT-00 | Record the authorized direction and preserve existing work | Baseline captured; work units and limits recorded | complete |
| CHAT-01 | Structured, bounded error accumulation and reports | CLI walkthrough retained `model_failed`, reloaded it, acknowledged it without deletion, and found no request text in reports | complete |
| CHAT-02 | Acknowledgment, resilient intake and mechanical fallback | A short receipt precedes AI; network/model/send failures preserve state without killing intake or replaying uncertain actions | in_progress |
| CHAT-03 | Process supervision, health and explicit stop | Abnormal exit/stall is detected and restarted; an explicit stop remains stopped; macOS/Linux service definitions are verified | pending |
| CHAT-04 | Chat operations and supported feature setup | Status/errors/jobs/pause/cancel/features/install remain usable without a working LLM; natural chat reuses typed host actions | pending |
| CHAT-05 | Simple installation and repair flow | Existing pairing reused; one guided enable path; external-host instructions distinguish installation, authentication and service readiness | pending |
| CHAT-06 | Integration and handoff | Relevant existing checks and actual failure/recovery walkthroughs; local commits; actual vs unperformed platform/live checks recorded | pending |

The OS service manager supervises a small chat supervisor. The receiver owns
transport retries and durable per-message receipts; a failed AI turn cannot stop
mechanical management commands. The controller remains the sole SQLite writer.
Error reports and process state use separately locked private host records.

An acknowledgment says only that the message was received, not that the requested
operation completed. No Telegram native read status or reaction is requested.
Uncertain message delivery and interrupted host actions are never automatically
replayed. Transient polling failures may reconnect with the saved cursor.

Error reports contain fixed public diagnostics, stage/classification, timestamps,
counts and correlation identifiers. Raw exception text, tokens, source contents,
conversation text and private reasoning are excluded. Repeated failures are
aggregated with bounded recent occurrences rather than growing without limit.

Supported built-in operations and assets form the installation/management
catalog. A guide is labelled as a guide; an unavailable connector is not called
installed. No arbitrary shell/download/install action is granted to the LLM.
Account grants and active Skill/evaluation/adoption controls retain their current
human ownership. First-host credentials use local protected input; chat never
asks a person to paste secrets into a provider conversation.

Explicit stop is a persistent desired state, separate from abnormal process
failure. Power loss, network outage, revoked credentials, a disabled OS service
manager and a broken/unwritable filesystem cannot be made continuously available
by application code. These conditions require retained diagnostics, bounded
recovery and a local/SSH repair path; no absolute uptime claim is made.

## Starting evidence

Committed baseline: `0ce3e41`. Existing uncommitted changes in Telegram and
Codex reception compatibility were captured privately before this work. Their
contents are preserved while the user clarifies whether another task is editing
the same files. No worktree, remote deployment or new test code is created.

The existing bot is paired. The prior check found no receiver process, only a
successful pairing receipt, and no retained terminal error log. Existing services
manage the job worker without automatic restart; they do not supervise Telegram.
The reception layer already provides typed actions and in-memory context, but
transport failures currently terminate its receiver. This goal supersedes those
foreground-only/recovery deferrals within the supported private-chat scope.

## CHAT-01 evidence

`go test ./internal/errorreport ./internal/cli` compiled the affected packages
(they contain no test files). A direct CLI walkthrough with an unconfigured AI
executor produced a retained `model_failed` report; a fresh CLI invocation read
it and `errors ack ID` retained history while acknowledging the occurrence.
The private error JSON did not contain the submitted message. Known diagnostics
are grouped by catalog code, with cumulative counts and a recent occurrence
limit from configuration. Temporary storage failures remain in a bounded
in-memory aggregate and are exposed as unpersisted counts in chat health.
No new test source files were added.

The pre-existing compatibility changes were independently committed as
`a421c19` before this goal's planning commit. Their reception compatibility policy
is preserved while prerequisite checks move to per-turn AI admission.
