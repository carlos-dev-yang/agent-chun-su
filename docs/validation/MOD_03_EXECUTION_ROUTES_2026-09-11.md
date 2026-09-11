# MOD-03 — roles, drivers, environments and recovery

The provider interface, environment boundary and role routing are now separate.
Codex retains the temporary verified version/model constraint. Runtime receipts
include role, driver, canonical read roots, no requested tool write roots, tool
network policy and argument digest. Existing configuration inherits the task
route; explicit reception/review settings are independent.

Actual macOS/arm64 checks:

- With the task model changed to an unsupported validation value, an explicitly
  configured reception route still generated a natural-language meeting title.
  An explicitly configured review route independently generated and imported
  evaluation `HWYJRUVBO6STU7BGKGFZ4SRINR` with role `review` and its own boundary.
- `doctor` distinguished task `needs_revalidation` from reception/review
  `prerequisites_match`. An unknown environment was rejected without changing
  the route. The task route was then restored to the verified selection.
- Synthetic no-new-target work completed as job `3723U4HQDOLI7FP2LBDQZSSFWQ`,
  attempt `HCVVFKCXDPDEH6EIIQSTY622NE`. Its preserved runtime evidence identifies
  role `task`, the package's canonical read root, empty tool write roots and
  disabled tool networking.
- A review parent was deliberately killed only after its running child identity
  was durably recorded. Controller recovery reconciled the surviving group,
  changed its process receipt to exited and recorded an interrupted review.
  No judgment was imported or automatically replayed.
- A separate backup/restore preserved review data and revoked approvals on
  explicitly configured reception/review routes. Synthetic approval flags used
  only for this offline restoration check were immediately restored in the
  source home; no live data was disclosed under them.
- A foreground review attempted while the chat setup host owned SQLite was
  correctly refused. It succeeded after that owner stopped. This remains a
  single-writer deployment, not concurrent independent controller ownership.

Existing runner/feedback tests passed; other affected packages compile without
test files. Affected vet and build passed. No new tests were created. Linux
package availability and a running local Linux/arm64 VM were established, but
Linux installation, native enforcement and service evidence belong to MOD-04.

See [the execution route contract](../contracts/execution-routes-v1.md). A matching
version is a prerequisite, not proof of company access or platform isolation.
