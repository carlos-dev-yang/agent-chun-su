# Chat-triggered updates from master

Implementation checkpoint: [UPDATE-01](../implementation/SELF_UPDATE_2026-09-19.md).
See [validation evidence](../validation/SELF_UPDATE_2026-09-19.md) for the checks
actually performed and the remaining external-delivery checks.

## One-time server configuration

Install the version that includes this capability first. As the same OS user
who owns the running services, register a clean local `master` checkout, its
existing `origin`, the installed build tools, and the full commit used to build
the installed application:

```sh
chunsu update configure \
  --repository "/path/to/agent-chunsu" \
  --go-path "/path/to/go" \
  --git-path "/path/to/git" \
  --installed-revision "FULL_COMMIT_OF_INSTALLED_BINARY"
chunsu update check
```

Replace the example paths and commit with verified local values. The command
records the installed executable's path and digest; run the installed `chunsu`,
not a temporary build. Reconfiguration is local-only. Do not infer the installed
commit from checkout HEAD after a self-update: updates leave the checkout intact
and record the installed revision separately. Existing Git authentication must
work without an interactive prompt. A Linux systemd user manager and Go/Git are
required; the updater does not install them or run the Ubuntu bootstrap.

The terminal equivalent of `/update` is `chunsu update apply`. It starts the same
independent service without sending a Telegram notification. The source/package
wrapper `scripts/update.sh` (packaged as `update.sh`) accepts `check`, `status`,
or `apply`; it does not accept a repository or branch.

## Owner workflow

Develop and validate a change, commit it, and push it to the registered
repository's `master` branch. In the paired bot's private chat:

```text
/update check
/update
/update status
```

Checking does not install. Applying retrieves one exact master commit and builds
it separately while the old chat is still running. A separate updater controls
the final restart, so closing/restarting the receiver cannot terminate the
update itself. When the activation gate finds active work, stop new dispatch,
allow the current work to finish and request the update again. Do not terminate
an active task merely to deploy an unrelated change.

An accepted request is separate from a completed update. `/update status`
distinguishes completion, rejected database changes, build failure, busy work,
successful rollback and recovery requiring the local terminal. Avoid concurrent
manual service/configuration changes while an update is activating. Active
activation rejects new mutations while leaving its status available.

The owner configures the source repository and build tool on the server once.
Chat cannot change the repository, branch, executable path, model or account.
The update command does not accept arbitrary URLs or shell commands. Existing
Git credentials remain in the owner's normal credential mechanism; never paste
them into Telegram or commit them to the repository.

## What stays the same

The private data home, saved task model, bot token, paired owner, reply language,
tone, reports and work history remain on the server. Explicitly stopped services
remain stopped. New code does not itself enable a stopped worker, unpause the
queue, enable schedules or approve disclosure of live data.

The supported rollout replaces the main application executable, checks startup and
restores the intended services. Telegram restarts at the end. A failed build
leaves the old services in place. A failure during activation attempts to restore
the previous executable and services; the durable status says whether recovery
succeeded or needs local intervention.

## Exclusions and fallback

A different database schema version or changed migration content is rejected
before candidate code opens the live database. Codex upgrades, new authentication,
OS package/security changes, secret-helper updates and database migrations require separate validation.
A rollback restores executable files; it does not overwrite current database
contents with an old backup or silently reconnect accounts.

A host outage or loss of all networking cannot be repaired by a bot command.
If the update cannot confirm recovery, inspect its saved status through the
server terminal. An ambiguous message delivery is recorded, not sent repeatedly.
SSM remains the independent way into the EC2 host.

Local diagnostics start with `chunsu update status` and
`systemctl --user status chunsu-update-REQUEST_ID`, using the saved request ID.
The private update record is under `state/self-update` in the application data
home; binary backups are under `.chunsu-update/backups/REQUEST_ID` beside the
installed executable. Do not remove a lock or activation marker while its updater
is alive. An interrupted activation requires an operator to verify/restore the
installed binary and prior service intentions before archiving stale update
state and configuring the verified installed revision again. There is no
automatic force-reset or database restore in this release.
