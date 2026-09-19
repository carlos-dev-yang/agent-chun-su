# Ubuntu EC2 installation

Run the following as the intended non-root Ubuntu user, from a trusted clone.
It installs only the Ubuntu build prerequisites that Chun-su needs, downloads
the exact Go toolchain named in `go.mod` and verifies it against the matching
SHA-256 entry in Go's official release manifest, builds a local checksummed candidate, and installs the matching
full Codex Linux platform package below `/usr/local/lib/chunsu` after verifying
the npm registry SHA-512 integrity value.

If the Ubuntu image does not include Git, install it before cloning, or transfer
an approved source archive instead:

```sh
sudo apt-get update && sudo apt-get install -y git
git clone YOUR_APPROVED_REPOSITORY_URL chunsu
cd chunsu
./install.sh --install-deps --enable-linger --prepare-codex --prepare-sandbox --model gpt-6-astra --start
```

The command asks the Codex CLI to sign in and, on a first setup, asks for the
Telegram bot token through Chun-su's protected local prompt. It then pairs the
bot, registers the Telegram user service, and, because `--start` was selected,
starts the managed controller and requested worker. Do not paste the token into SSH
history, shell environment variables, source files, or an AI conversation.

`--model` is optional for a new home. Without it, the installer lists the model
IDs supported by the built-in metadata, marks the configured default, and asks
for an explicit choice. The `gpt-6-astra` entry uses its validated low-reasoning
preset. A later run preserves the selected route and stopped service intent.
`--start` explicitly starts a paired Telegram service and the managed controller
and worker; omit it to preserve a prior stopped intent.

The installer does not configure AWS networking, SSH access, firewalls, public
ports, or global system security policy. Telegram uses outbound polling and
needs no inbound listener. `--enable-linger` is an explicit administrator-approved
option that enables the current user's persistent systemd user manager after
logout or reboot:

```sh
./install.sh --enable-linger
```

Review the Codex executable and any Ubuntu AppArmor user-namespace rule with
the host administrator. The installer leaves the system-wide user-namespace
restriction unchanged.

`--prepare-sandbox` is an explicit host-administrator choice. It loads one
root-owned AppArmor profile for the exact verified Codex executable and permits
only its user namespace setup; it does not change a global sysctl, disable
AppArmor, or apply to another executable. Ubuntu documents this
[per-application user-namespace approach](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/userns-creation/).

## Check recovery without changing anything

The recovery tool reads the configuration and service status, and writes a
JSON report outside the checkout. Its default mode never starts, stops, or
restarts a service.

```sh
./scripts/check-recovery.sh
```

Use the opt-in exercise only while the controller and any active worker are
idle. It restarts only services that were already running, checks their
converged status, compares a canonical configuration digest, and leaves
services that were explicitly stopped stopped.

```sh
./scripts/check-recovery.sh --exercise
```

The report is stored under `$XDG_STATE_HOME/chunsu/recovery` or
`$HOME/.local/state/chunsu/recovery`. It contains service status, not tokens.

Python is used only by the installer, release-packaging, and recovery helpers.
The installed Chun-su controller and services run as Go binaries; they do not
require a Python runtime.
