# INSTALL-01 — EC2 first installation and recovery checks

Status: implemented and validated locally on macOS and an isolated Ubuntu VM.
Actual EC2 deployment, Telegram pairing, and host reboot acceptance remain pending.

The user authorized a clone-based installation flow, first-install model selection,
automatic Telegram credential-store initialization, and repeatable recovery checks.
The selected new model is `gpt-6-astra` with `low` reasoning effort; the user's
earlier `6-astra-light` wording was clarified explicitly. Existing installations
retain their saved routes unless the owner requests a change.

## Scope

- A source-checkout installation entry point builds a checked release package,
  installs the application/helper, and guides first configuration and authentication.
- Ubuntu host prerequisites and persistent user services are prepared through an
  explicit installer option or interactive selection. No global sandbox weakening,
  public management port, EC2 resource creation, or root application service.
- Installed commands expose shared executor/model compatibility metadata. The new
  model retains the existing restricted execution boundary and receives explicit
  low reasoning. Existing gpt-5.5 configurations remain supported.
- First Linux Telegram connection prepares a local encrypted credential store
  when no external store is selected. Later invocations reuse it. Missing or unsafe
  existing keys fail clearly rather than silently replacing credentials.
- Recovery tooling distinguishes read-only readiness from explicit service restart
  exercises. A data backup restore remains different from process restart and does
  not claim to restore external credentials.

## Evidence required

Record focused existing Go checks, script syntax and package checks, isolated
Linux credential initialization/reuse/failure checks, service restart persistence,
and a real synthetic report using the selected model. Record unperformed EC2,
Telegram DM, native architecture, and reboot checks separately. Runtime credentials,
private settings, generated reports, and binaries remain outside Git. Publishing
the local commits remains a separate user instruction.

## Completion evidence

See [focused installation validation](../validation/EC2_FIRST_INSTALL_2026-09-19.md).
The source installer, model metadata/preset, automatic Linux credential storage,
and recovery helpers are delivered together. Existing saved routes are preserved.
