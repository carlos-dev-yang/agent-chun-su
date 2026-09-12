# Persistent chat operations v1

Authorized by the user's 2026-09-13 request. This extends, and supersedes the
foreground-only failure behavior in, [Telegram v1](telegram-v1.md) and
[natural chat v1](chat-v1.md). There is no SQLite schema change.

## Responsibilities

`internal/reception` owns conversation context, allowed actions, fixed commands,
host dispatch and selected-process recovery. `internal/telegramchat` owns the
transport interaction, receipt-before-execution, acknowledgment, cancellation and
per-message failures. The CLI handles setup and service lifecycle. The reception
AI receives no native tools; task workers retain their separate source and output
gateways. The CLI/model compatibility policy remains inside the initial driver.

`telegram enable` pairs locally when necessary, installs a user service and saves
enabled intent. macOS launchd or Linux systemd supervises `telegram supervise`;
that process supervises one receiver. Linux uses `Restart=on-failure`; enabled
supervision interrupted by a signal returns failure, while an explicitly stopped
home exits normally. The receiver has its own Telegram lock and
an event-loop heartbeat. OS supervision restores a crashed supervisor. The app
restarts an exited/stalled receiver with bounded backoff, retaining process
identities and error records. Cleanup verifies process start identity; an
unidentified surviving group blocks unsafe replay and remains reported.

The controller remains the only SQLite writer. Reception reuses an existing
controller, or maintains its own separately identified setup controller. The
setup controller reconciles interrupted task processes but executes no queue or
schedule. `/worker start` requests a switch of that owned process to a queue
worker; `/worker stop` switches back to setup management. Worker intent survives
receiver restart. A separately launched worker is never terminated by this
switch; `/pause` and `/cancel ID` still reach its controller. Starting a worker
does not unpause the queue, activate schedules or change disclosure approvals.

## Messages and failures

Only the paired human's private, non-forwarded text is admitted. A durable
receipt is created before advancing the saved cursor. Ordinary requests receive
the fixed text `접수했습니다.` before any LLM call. Fixed commands reply directly.
No native read action or reaction is used. Acknowledgment means receipt only.
If acknowledgment delivery is unconfirmed, that request does not begin AI work.

Polling reconnects from the saved offset. Backoff uses the configured polling and
retry limits and respects Telegram `retry_after`. Authentication failures reopen
the selected secret store; restoring the original bot token can take effect on
the next attempt. Webhooks are preserved, never deleted. Outbound messages are
serialized and spaced; failed or uncertain sends are not automatically repeated.
Network failure, failed generation, timed-out generation, failed host action,
uncertain reply and interrupted request are separate operational events.

One AI turn runs at a time. Fixed commands remain available during AI work. A
second ordinary request receives a busy response, rather than silently queuing.
Cancel requests interrupt the generation; a new AI turn waits for cleanup.
Whole remote turns have the configured time limit, including multiple AI steps.
An uncertain result triggers process reconciliation and never task replay.
Orphaned execution blocks further AI work until reconciled; commands remain
available. `/reset` starts fresh context after recovery. Restarted conversation
history is empty: no transcript is persisted or automatically replayed.

Receipts record the acknowledgment separately from the final reply, with message
IDs, hashes, update/session IDs, timestamps and an error-group ID. Interrupted
claims are reported on restart. A durable claim is never proof of completion.

## Operations and installation

Both deterministic commands and AI-proposed actions use the same host validation.
The contract adds error list/acknowledgment, feature list/install, queue pause and
resume, selected job cancel/retry and Telegram-owned worker start/stop. Job and
error IDs are validated; operations do not gain arbitrary filesystem paths,
shell commands, destinations, credentials or evaluator-adoption authority.

`/features` distinguishes compiled workflow modules from setup manuals.
`/install workflow-ID` installs/validates the built-in workflow bundle, preserving
an existing active selection. `/install guide-SERVICE` installs public guidance,
not an external connector. Unsupported IDs fail rather than downloading code.
New connectors/drivers still require implementation, bundling and boundary
validation; account grants and secret entry remain local or via approved SSH.

## Error reports and recovery limits

`state/errors/` contains one private atomic report per fixed diagnostic code,
with cumulative count, first/last timestamp, bounded recent opaque correlations
and acknowledged count. Its own lock never writes SQLite. There are no raw
error strings, conversation contents, credentials or provider response bodies.
Unpersisted errors remain in a bounded in-memory aggregate and are exposed in
health; later writes retry them. A damaged report stays on disk and is shown as
unreadable while other groups remain inspectable. This cannot durably retain
new data during power loss or a completely unavailable filesystem.

`/errors`, `chunsu errors` and `errors show ID` report the retained facts. `/ack ID`
and `errors ack ID` acknowledge current occurrences without deleting history or
claiming the cause is fixed. Foreground startup diagnostics print a safe error
ID; supervised processes discard raw subprocess stderr and retain catalogued
diagnostics instead. Existing metadata identifies the associated reception
session, update, process and controller action for local investigation.

`telegram stop`/`disable` removes enabled intent before stopping the service.
That stop survives login/reboot; `start` explicitly re-enables it. `remove` also
unregisters the service and preserves data and diagnostics. Direct SIGKILL is an
unexpected failure and is recovered; use the stop command for a durable stop.

The process cannot remain reachable through a powered-off/sleeping host, a
network outage, revoked credentials, an unavailable OS user manager or a broken
filesystem. Linux boot operation needs an approved persistent user manager;
macOS LaunchAgents start in the user session. If Telegram itself is unavailable,
use local/SSH `telegram status`, `errors`, `doctor`, `telegram restart`. State
files are private operational records, not secrets or a network management API.
Current backups do not include Telegram bindings, chat metadata, error reports or
supervision intent; retain needed diagnostics separately. Restoring report data
does not enable chat/worker intent or restore secret values.

Sources: [Telegram Bot API](https://core.telegram.org/bots/api),
[Telegram FAQ](https://core.telegram.org/bots/faq),
[Apple launchd guide](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html),
[systemd service specification](https://github.com/systemd/systemd/blob/main/man/systemd.service.xml).
