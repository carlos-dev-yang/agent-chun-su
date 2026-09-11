# Install Chun-su locally or on a Linux server

Chun-su uses one Go executable and an embedded SQLite engine. The same private
data-home contract applies on a local computer, a company-managed workstation
or a Linux/EC2 host. Each data home has one controller owner. Do not put its
SQLite database on shared storage for multiple worker/controller writers.

## Build a local release candidate

From the checkout with the selected Go toolchain and Python 3:

```sh
python3 scripts/package.py --version local-candidate --target darwin/arm64 --target linux/arm64 --target linux/amd64
```

Packages contain binaries, public examples/evaluator Skills, documentation,
dependency versions/licenses and SHA-256 checksums. They contain no account
credentials or runtime data. Manifests distinguish a dirty working tree from
the recorded commit. These are unsigned local candidates, not public releases.
A successful build alone does not establish platform enforcement or account
access. See [the final integration record](../validation/MOD_07_INTEGRATION_2026-09-12.md)
and [Linux boundary evidence](../validation/MOD_04_SERVER_2026-09-11.md) for
the checks actually performed. Public manuals, referenced planning/validation
records and Skills are included so their local links work after extraction.

## Install on the selected host

Transfer the matching archive through your approved channel, extract it, and
run its installer as the intended OS user:

```sh
./install.sh
# Optional, when this host will use the bundled encrypted credential helper:
./install.sh --with-secret-store
```

The installer verifies checksums and the host OS/architecture. It installs into
`$HOME/.local/bin` by default; `--prefix ABSOLUTE_DIRECTORY` selects another
prefix. Ensure its bin directory is on PATH. It preserves application data and
does not install or start a service. The runtime needs no Go compiler, Python
or SQLite CLI. The AI driver remains a separate prerequisite.

```sh
export CHUNSU_HOME="$HOME/.local/share/chunsu"
chunsu setup
chunsu config route task --driver codex --path "$CODEX_PATH" --model gpt-5.5 --environment native-restricted
chunsu doctor
chunsu queue examples/mail/synthetic-day.json
chunsu run JOB_ID
chunsu report JOB_ID
```

Use an absolute `CODEX_PATH` for the separately installed pinned CLI. Authenticate
with the driver's own login command on that host. Do not copy someone else's
CLI account files into a server image. On macOS the data home must be outside
system temporary directories due to the verified native sandbox behavior.
Linux requires the native sandbox prerequisites to pass on the selected kernel
and host policy; version matching in `doctor` is not an enforcement test.
The Linux boundary was checked with the complete driver distribution installed
in a root-owned, readable directory under `/usr/local/lib`. A driver placed
inside a private home directory may not be readable by its own restricted
child process. Keep program files separate from account files and task inputs.
On Ubuntu 24.04, the administrator may need an AppArmor user-namespace rule
for the exact driver executable. The validation VM used that specific rule;
it kept the system-wide user-namespace restriction enabled. Have the host
administrator review the executable and rule; the installer does not weaken
system security settings. See [Ubuntu's explanation](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/).

## Connect a server credential store

Use an approved helper or the optional encrypted helper. For the latter,
provision a private 32-byte random key separately and configure:

```sh
export CHUNSU_SECRET_HELPER="$INSTALL_PREFIX/bin/chunsu-secret-store"
export CHUNSU_SECRET_KEY_FILE="$MASTER_KEY_FILE"
export CHUNSU_SECRET_STORE_DIR="$ENCRYPTED_SECRET_DIRECTORY"
chunsu gmail secret-check
```

All expanded paths are absolute. Provision them through the host's secret
management process; never put key bytes into these variables or CLI arguments.
See `CREDENTIALS.md` in the package or the
[credential contract](../contracts/credential-helper-v1.md) in the checkout.
An existing company vault can replace the helper while leaving the core intact.

Gmail/Jira/Telegram connections still need the account scopes and node access
selected by the owner. For a remote Gmail loopback callback, use the documented
local authorization flow on an approved workstation or a correctly scoped SSH
port forward; a URL referring to the server's loopback is not your browser's
loopback. A dedicated managed workstation is useful when document protection
requires that device identity. Installing on a VPC/EC2 host supplies a network
location; the configured user/service identity and permitted model route still
determine access and disclosure.

## Run and manage a service

Foreground execution works without a service manager:

```sh
chunsu worker
```

On macOS use a user LaunchAgent; on Linux use a systemd user manager with its
session bus available. Minimal Ubuntu images may need `dbus-user-session` and
a restarted user session before `systemctl --user` can operate:

```sh
chunsu service render
chunsu service install
chunsu service start
chunsu service status
chunsu service stop
chunsu service remove
```

Only `service install --at-login` opts into future user-session startup. Linux
boot operation without an interactive login requires the administrator's
approved user-manager/lingering configuration. The application does not enable
lingering, open public management ports or create a privileged service account.
Without a user manager, run the foreground worker under your existing approved
supervisor. Service definitions capture selected non-secret host paths, including
the helper paths; they do not capture arbitrary tokens or shell startup files.

The management socket remains local to the private data home. Use an authenticated
SSH session as that OS user for remote commands. A second writer is refused.
The reception and task AI processes remain separate from this controller.

Use `chunsu access status` to inspect actual OS ownership and execution access.
`chunsu access revoke` pauses work, cancels running agents and removes source
disclosure approvals; `access resume` leaves disclosure and queue decisions for
explicit review. [Owner access](../contracts/owner-access-v1.md) describes the
matching-user socket, audit receipts and the supported deployment boundary.

## Update and recovery

Stop the existing controller/service, make a `chunsu backup` in a separate
directory, and verify it with `chunsu verify-backup`. Install the new candidate
over the same binary location, then run `doctor` and `recover` before resuming.
The installer does not alter data or silently migrate it. Supported schema
handling remains in the application; an unsupported newer schema fails rather
than being reset. If the service executable path changes, remove/reinstall its
registration instead of editing a hash-verified definition behind the application.

Restore into a fresh data home with `chunsu restore BACKUP NEW_HOME`. Review
the restored settings and reconnect accounts; all route disclosure approvals
are revoked. Review data is included but secret values/master keys are excluded.
Do not merge two independently modified SQLite homes. Keep a backup produced
before an incompatible migration for rollback; replacing a binary alone cannot
undo a schema migration. Removing installed binaries or a service preserves data.
