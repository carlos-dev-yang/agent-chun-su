# MOD-06 — Owner identity, revocation and local audit

Declared single-owner scope complete on top of `03b0ed7`. No schema migration,
shared database server, organization IdP or public management endpoint was added.

## Changes and evidence

- Data/state directories, config, SQLite and writer lock require matching OS
  ownership and private permissions. Secret-key ownership is also checked.
- Unix management connections verify the peer at both ends. On an actual
  Ubuntu arm64 user service, the matching user obtained controller status. A
  root process that could bypass filesystem DAC still received no response to
  a raw management request: the server rejected the different peer UID. The
  matching user continued to obtain status afterward.
- The Linux CLI refused another user's data home even when invoked through
  sudo; a broadly readable configuration was refused and its private mode was
  restored. Existing macOS owner commands and private socket operations passed.
- `access revoke` cancelled actual macOS mail task
  `DQC4A3YVKWPCD5ZZHX5A7GREQB`, attempt `IOW6PGSXN5R37VPL4WTWTG4R4H`.
  The actual CLI process ran for about 15.9 seconds, had used only the allowed
  mail source capability, and persisted an interrupted result and exited
  process identity. The job became cancelled, the queue paused, source
  approvals cleared and the persistent execution block remained.
- A requested independent review while blocked returned before its provider
  invocation. Explicit resume removed the block while preserving the queue
  pause and revoked approvals. A second actual check revoked execution while
  a separate reception AI process was running; its process record reached
  exited through the driver's cancellation path at the configured poll interval.
- Owner-derived audit receipts were observed for config preparation and access
  actions; record preparation and successful control selections use the same
  mechanism. Receipts do not turn an AI/validation actor into a human reviewer.
- The Linux native sandbox also ran under a transient systemd user service
  with `NoNewPrivileges=yes`. Package reading succeeded and access to the live
  management socket failed. Global Ubuntu namespace restrictions stayed enabled.

Existing runner/feedback tests and affected store/files/platform/control/backup/
retention/config/secret/CLI builds passed. Focused vet and whitespace checks
passed. No test code was added. Linux arm64 cross-build and actual binary
execution passed; final archive checks are in MOD-07.

## Verification corrections and scope

The first manual cancellation script polled for an incorrect process-state
label (`started` instead of `running`) and checked an omitted false JSON flag as
if it were required to be present. The actual preserved record showed a real
interrupted process and cancelled job; the corrected postconditions passed.
This script issue was not rewritten as a product failure or a successful polling
check. The reception cancellation check observed the correct running state.

The contract is [owner access v1](../contracts/owner-access-v1.md). Same-user local
management is trusted; it is not multi-user SSO, physical-human attestation or
tamper-resistant remote audit. No company account, protected document or real
EC2 host was available. Existing local process evidence and the separate Linux
VM checks establish the declared installation/owner boundary only.
