# Persistent chat operations checkpoint — 2026-09-13

Implementation and candidate packaging complete. Live bot handoff awaits macOS
Keychain access; the earlier receiver is preserved. This is a single-owner
local/Linux server candidate, not an enterprise SSO or production EC2 deployment.

## Implemented result

- Short fixed acknowledgment before AI; no Telegram native read receipt/reaction.
- Reusable reception and Telegram receiver, fixed management commands during AI
  failures, bounded remote turn time, cancellation and selected-process recovery.
- Separately locked, bounded structured errors; retained counts/correlations,
  unreadable-file isolation, review acknowledgment without deletion.
- Poll reconnect/backoff and retained cursor; no replay of uncertain sends/actions.
- Host-owned catalog and workflow/manual installation; active controls preserved.
- Optional owned worker start/stop; one SQLite writer, default worker intent off,
  explicit source approvals and queue/schedule state preserved.
- OS user-service supervision, heartbeat watchdog, persistent explicit stop,
  safe process identities, observed health and read-only bot credential check.
- Pair/enable/check/start/stop/restart/remove plus installer opt-in and server guide.

## Checks actually performed

| Check | Evidence and result |
|---|---|
| Existing Go checks | `go test` for runner and feedback passed; affected reception, Telegram, service, supervisor, CLI/error packages compiled. `go vet` passed for affected packages. No new test source files were added. |
| Actual macOS Codex reception | Existing Telegram walkthrough passed acknowledgment-before-AI, follow-up context, guide installation/status, rejected unpaired sender, duplicate suppression and cancellation. |
| Broken AI | Fixed error lookup, module installation, pause/resume and owned-worker start/stop operated with an unusable AI executable. |
| Failed send | One unconfirmed send receipt/error retained; receiver remained alive and processed later commands. |
| Error persistence | Fresh CLI read the accumulated failure, acknowledged it without deletion and found no request text in error JSON. Malformed report contents were withheld while a fixed unreadable report was returned. |
| Network loss | Native Linux arm64 + existing local Bot API fixture + isolated encrypted fixture store: stopped API server, retained `poll_failed`, kept receiver alive, restarted API server and delivered subsequent `/help`. |
| macOS supervision | Killed receiver restored; killed supervisor restored by launchd; SIGSTOP-stalled receiver restored by heartbeat watchdog; explicit stop and removal succeeded. |
| Linux supervision | systemd user manager restored killed receiver/supervisor; SIGTERM while enabled was recovered; explicit stop became OS `inactive`, stayed inactive after VM reboot and removal succeeded. Generated unit passed `systemd-analyze --user verify`. |
| Installation | macOS arm64 and native Linux arm64 archives installed and initialized in separate private homes. Optional helper installed on Linux. Linux amd64 core version command executed through emulation. |
| Packaging | Clean source revision, checksums, licenses/public assets and linked setup documents verified. No runtime databases or credentials included. |

Linux boot validation initially found a restart loop after explicit stop and a
failure to remove an `activating` service. The final implementation uses
`Restart=on-failure`, returns failure for an interrupted enabled supervisor,
recognizes transitional OS states and was rechecked through an actual VM reboot.
The application stop flag alone was not accepted as evidence of OS inactivity.

## Candidate artifacts

Source: `9fc4a79926136c9f27a6f625973c9cd2a8334499`, working-tree changes: false.
Version: `chat-ops-20260913-rc1`. Archives are local and unsigned; no remote publish.

| Archive under `dist/` | SHA-256 |
|---|---|
| `chunsu-chat-ops-20260913-rc1-darwin-arm64.tar.gz` | `69581a9da090cb7e43291afa204601da611cb50b1dc3eebd35347e377ae4bf36` |
| `chunsu-chat-ops-20260913-rc1-linux-arm64.tar.gz` | `358c6b687d53e9ff6acec0e9a7ac6bcb5f96ab9a7ddd11a94523171f298c5bfc` |
| `chunsu-chat-ops-20260913-rc1-linux-amd64.tar.gz` | `bfe7baf4ff48205bd7762a7978f62195870e9deae40c94443dae04a08ad00f51` |

## Remaining live handoff and limits

Later macOS fixture creation was rejected twice by Keychain (`errSecAuthFailed`).
A read-only check of the real saved bot token was also rejected (`security exit
51`). That access path was not retried or bypassed. The user was asked to restore
OS Keychain access. The old receiver remained running and its token/ownership
were not changed. The newly built checkout binary is ready, but does not replace
an already-running process image.

After access is restored, run `bin/chunsu telegram check`, gracefully stop the
existing foreground receiver, run `bin/chunsu telegram enable`, then verify
`telegram status` shows live supervisor/receiver and connected polling. A short
DM from the paired user should receive the acknowledgment and AI response.
This live handoff and the new receiver's real Telegram delivery remain pending.

The same Keychain prerequisite remained unresolved through the original goal
turn and two consecutive continuations. Read-only process/status checks still
found the legacy receiver alive and the new supervisor disabled. With independent
implementation and validation finished, the goal is blocked pending the user's
OS access recovery; it is not complete. No further credential access was retried.

No real EC2/company network, protected document system, native Linux amd64 AI
boundary, full macOS logout/reboot, global filesystem failure, provider-wide
outage or universal uptime guarantee is claimed. Native Linux service and
network behavior were checked in the isolated Ubuntu arm64 validation VM.
Source/model permissions, existing compatibility limits, human-owned criteria
and optional result evaluation stay in effect. Report backups do not include
chat/Telegram/error/supervision records; retain needed diagnostics separately.
