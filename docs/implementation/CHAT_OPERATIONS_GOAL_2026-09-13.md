# Persistent chat operations goal — 2026-09-13

Status: blocked on live handoff pending OS Keychain access.

The user explicitly requested short mechanical acknowledgments,
accumulated internal error reports, chat-based management and supported feature
installation, recovery from channel/process failures, supervised operation until
explicitly stopped, and simple local/server installation.

## Direction and completion evidence

| Unit | Work | Completion evidence | Status |
|---|---|---|---|
| CHAT-00 | Record the authorized direction and preserve existing work | Baseline captured; work units and limits recorded | complete |
| CHAT-01 | Structured, bounded error accumulation and reports | CLI walkthrough retained `model_failed`, reloaded it, acknowledged it without deletion, and found no request text in reports | complete |
| CHAT-02 | Acknowledgment, resilient intake and mechanical fallback | Live Codex acknowledgment/context/cancel passed; broken-AI and unconfirmed-send flows passed; Linux API disconnect/reconnect preserved intake and replied after recovery | complete |
| CHAT-03 | Process supervision, health and explicit stop | macOS exit/stall recovery; Linux exit/SIGTERM recovery, explicit stop, reboot staying OS-inactive and removal passed | complete |
| CHAT-04 | Chat operations and supported feature setup | Fixed commands, workflow installation, queue controls and owned worker start/stop passed with a broken AI executable | complete |
| CHAT-05 | Simple installation and repair flow | Clean-revision packages for macOS arm64/Linux arm64/amd64; installed macOS/Linux arm64 and verified linked guides and checksums | complete |
| CHAT-06 | Integration and handoff | Implementation, native checks and packages complete; existing bot supervisor handoff awaits OS Keychain access | blocked |

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
Codex reception compatibility were captured privately before this work and
subsequently committed as `a421c19`. They were preserved throughout this goal.
No worktree, remote deployment or new test code was created.

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

## Current integration boundary

The existing bot receiver remains running on the earlier binary. macOS now
rejects new Keychain writes (`errSecAuthFailed`) and the read-only `telegram check`
also rejected access to the existing token (`security exit 51`). No token was
extracted, copied from process memory, logged or bypassed. The user has been
asked to restore OS Keychain access before the live supervisor handoff. This is
a deployment prerequisite, not a reason to stop implementing independent units.

Linux boot validation exposed an `activating/auto-restart` loop after explicit
stop and removal rejected that transition. The implementation now uses
`Restart=on-failure`, treats activation/deactivation as service states and stops
them explicitly. The corrected native validation passed; see CHAT-03 below.

## CHAT-02 / CHAT-04 evidence

The existing Telegram walkthrough used the actual macOS reception Codex route:
short receipt before AI, multi-turn context, host guide installation/status,
unauthorized sender rejection, duplicate suppression and cancellation passed.
A separate walkthrough with a deliberately unusable AI executable kept error
lookup, built-in workflow installation, pause/resume, owned worker start and
owned worker stop available. An unconfirmed send was retained exactly once and
the receiver stayed alive. The workflow's existing active selection was reused.

A Linux arm64 walkthrough used the existing local Bot API fixture and an isolated
encrypted fixture credential store. Stopping the API server retained a
`poll_failed` report without exiting the receiver; restarting it on the same
address delivered a subsequent `/help` response. No live company/source input
or real bot token was used. New test source files were not added.

Related existing runner/feedback tests passed. The reception, transport,
service, supervisor and CLI packages compiled and passed `go vet`.

## CHAT-03 final native checks

macOS launchd restored a killed supervisor; the supervisor restored a killed
receiver and a SIGSTOP-stalled receiver. Explicit stop and removal succeeded.
Linux systemd restored a killed supervisor and the corrected implementation
restored one interrupted by SIGTERM while enabled. The service was explicitly
stopped, the validation VM rebooted, and both app intent and actual systemd
state stayed stopped (`inactive`); removal then succeeded. `systemd-analyze
--user verify` accepted the generated definition. The earlier restart loop and
transition-removal failure were corrected rather than ignored.

## Installation handoff

Candidate `chat-ops-20260913-rc1` was built from clean source `9fc4a79` for
macOS arm64, Linux arm64 and Linux amd64. macOS and native Linux arm64 package
install/setup/error inspection passed. The Linux amd64 core executable ran via
emulation; no native amd64 AI/sandbox claim is added. New guide links resolved
and a malformed error record stayed inspectable without echoing its contents.
The checkout's `bin/chunsu` now contains the candidate. The already-running
legacy receiver still uses its original process image and was not interrupted.

See [the integration and handoff record](../validation/CHAT_OPERATIONS_2026-09-13.md).

## Resume CHAT-06

| Item | Current checkpoint | Next evidence required |
|---|---|---|
| Implementation and packages | CHAT-00–05 complete; `chat-ops-20260913-rc1` available locally | Existing validation and package hashes are linked above |
| Host credential access | The saved bot token read failed with `security exit 51`; the underlying OS authentication cause is unresolved | User restores Keychain access and `telegram check` succeeds |
| Running bot | Earlier foreground receiver was still alive at the last read-only check; new supervisor was disabled | Fresh process ownership check, then a single supervised receiver with connected polling |
| Live delivery | New behavior passed the local Bot API walkthroughs; the updated receiver has not been checked against the real bot | Paired owner receives the short acknowledgment, AI response and fixed-command results |

1. The user restores Keychain access on the Mac. Do not retry the denied access
   without a changed condition, extract the token from the live process, or
   replace the secret store to bypass OS denial. Token values and passwords
   remain outside chat and Git.
2. From the checkout, run `bin/chunsu telegram check` using the existing data
   home. This checks the saved token/bot without consuming updates. If it still
   fails, keep the existing receiver running and retain the safe diagnostic.
3. After the check succeeds, verify the current foreground process identity and
   stop that receiver gracefully. Do not reuse a PID from an earlier checkpoint.
4. Run `bin/chunsu telegram enable`, then `bin/chunsu telegram status`. Verify
   `enabled`, `supervisor_alive` and `receiver_alive` are true and polling is
   connected. Use the same configured data home throughout; do not create a
   second receiver with a different home for this bot.
5. Have the paired owner send one ordinary DM, followed by `/status` and `/errors`.
   Verify acknowledgment before the AI reply, working fixed commands and retained
   diagnostics. Inspect delivery metadata without publishing conversation text.
6. Record the real handoff evidence, then complete CHAT-06 and the goal. Source
   push or a successful credential check alone does not complete this step.

The remaining live handoff does not block source/document publication. Actual
EC2/company deployment and the other unverified environments listed in the
validation record are separate follow-up validation scopes.
