# INSTALL-01 — installation and model validation

Status: focused local validation completed; not an EC2 deployment claim.

## Linux credential initialization

Actual local Ubuntu 24.04.1 arm64 VM, kernel 6.8.0-50-generic, systemd user
manager available. Both Go commands were cross-built and copied to a dedicated
private Linux directory. No host account credentials were copied.

- First `telegram pair` with a deliberately absent local token file initialized
  the full data home and SQLite database, then stopped at token input validation.
  Automatic helper metadata, a 32-byte 0600 random key, and private ciphertext
  storage existed. No Telegram API call or real token was involved.
- A fresh CLI invocation with no helper environment variables passed
  `gmail secret-check` using the persisted paths. The key digest was unchanged.
- Removing the saved key refused secret setup without recreating it; config
  metadata, doctor, and controller status remained available.
- With a synthetic encrypted record present, removing both metadata and key
  still refused key regeneration. Ciphertext did not contain the test plaintext.
- Backup verification and restore into a fresh home passed. No
  `state/credential-store` entry appeared in the backup manifest or restored home.
- Independent Terra review found no blockers in this change set.

## Astra low reasoning

The user clarified `gpt-6-astra` with `low` reasoning. The official model guide
documents the exact ID and low effort; the local model cache identifies Astra's
tool mode as `code_mode_only`.

- Real reception invocation read the saved worker configuration and answered in
  Korean. The recorded host action was `read_worker_config`; no configuration
  changed and the tool-free reception boundary was retained.
- The first synthetic mail run successfully contacted Astra but could not call
  the source tool while code mode was disabled. The host correctly rejected it
  with `required_source_lookups_missing`. This was not a successful report and
  is retained as migration evidence rather than labeled a transient retry.

- After enabling Astra's required code-mode transport and supplying the full
  Codex distribution, an explicit retry completed the synthetic report.
  All seven observed tool calls were `chunsu_mail.mail_source_get`. The host
  verified schema, pinned time, source existence, target coverage, bidirectional
  source mapping, and known gaps. No semantic quality evaluation was performed.
  The execution boundary retained no writable roots and no tool network access.
- A standalone Codex binary with no discoverable code-mode host is rejected
  by task-route inspection before work starts. Reception remains available.
  The full distribution passed task prerequisites. These checks used a sanitized
  PATH so unrelated installed helpers could not satisfy the prerequisite.
- Actual model invocation evidence is macOS arm64 only, not Linux model acceptance.

## Ubuntu installer and process recovery

- The source installer ran on Ubuntu 24.04.1 arm64, downloaded the Go version
  from `go.mod`, verified its official manifest SHA-256, and built/installed both
  application binaries. A first explicit gpt-5.5 route and a separate first
  install selecting the default gpt-6-astra route succeeded.
- The complete Codex 0.153.4 Linux platform distribution was checked against its
  npm SHA-512 integrity and installed root-owned. The executable-specific
  AppArmor profile loaded successfully, and the native named-policy sandbox
  probe passed. Global `apparmor_restrict_unprivileged_userns` remained `1`.
- Login and Telegram setup were explicitly skipped in source-install validation;
  no host provider credentials were copied into the VM.
- Actual controller restart restored an already requested worker. The recovery
  report confirmed unchanged configuration digest and unchanged service intent.
- A second exercise restarted the controller and independent monitor while
  preserving an explicitly stopped worker. Unconfigured Telegram was untouched.
- Re-running the source installer while the controller was active preserved the
  existing Astra route even with a different first-install `--model` argument,
  and preserved the stopped worker. The subsequent read-only recovery check passed.
- Independent Terra review of the installer and recovery changes found no material blockers.
- Recovery fixtures cover omitted empty active jobs, delayed readiness, busy
  refusal, command errors, degraded services, stopped-but-live processes, and
  unconfigured Telegram. Reports use unique names and private permissions.

## Scope of checks

Existing Go tests for secrets, backup, CLI, service, error reporting, executor,
and runner passed. Runner has actual tests; the other listed packages currently
have no test source, so their `go test` result establishes compilation only.
A final repeat of the scoped Go tests and `go vet` passed after the host-prerequisite change.
Shell syntax and the recovery fixture suite also passed.

No actual EC2 host, real Telegram pairing/DM exchange, protected live source,
or production runtime upgrade has been performed for this work unit.

Actual EC2 provisioning, Linux model authentication/inference, real Telegram
DM exchange, and host reboot recovery were not tested. Process restart validation
does not establish reboot or end-to-end external delivery acceptance.

Linux arm64 and amd64 release candidates built successfully. All package file
SHA-256 values and required recovery companions were verified. The packaged
recovery entry point also passed against the actual Ubuntu services. amd64 was
cross-built, not executed. Validation services and the initially stopped VM were
stopped after testing; the existing macOS production runtime was unchanged.
