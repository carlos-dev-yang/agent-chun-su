# MOD-07 — Local/server integration and candidate handoff

This checkpoint records the bounded technical outcome of
[MOD-00–07](../implementation/MODULAR_AGENT_GOAL_2026-09-11.md). It is a private,
single-owner installation candidate, not acceptance of personal reporting
quality or a claim of organization-wide shared use.

## Delivered boundaries

- One Go controller owns a private SQLite data home. Reception, task execution
  and on-demand review use separate contexts/processes and configurable routes.
- Drivers and execution environments have separate interfaces. The initial
  Codex 0.153.4 / GPT-5.5 restriction remains a temporary verified combination;
  another driver or model is not enabled merely by changing a name.
- Skills, golden cases, rubrics, analyzer/optimizer criteria, selected reviews
  and adoption requirements are versioned. AI results cannot activate controls.
  Host contract/source/execution checks apply without compulsory AI grading.
- Mail, Jira and read-only Git review reuse the workgroup lifecycle. Code input
  is an immutable capture of explicitly selected committed files; the model
  cannot execute the repository or modify its checkout.
- The node uses actual OS ownership and local socket peer identity. Revocation
  pauses admission, cancels agents, clears disclosure approvals and persists
  across restart. Resume does not silently restore the earlier permissions.
- macOS Keychain or a separately selected credential helper, portable archives,
  an optional Linux systemd user service and fresh-home recovery support local
  and server operation without a separate database service.

## Environment and support matrix

The runtime checks below used candidate `modular-20260912-rc1` built from clean
commit `7eb57100142f87c845b00931713402ff7e5070e6`. The subsequent packaging change
includes all referenced public manuals and Skills; it does not alter Go runtime
behavior, the active Skill, result schema or disclosure defaults. Each archive's
manifest records its own source revision and dirty-tree state.

| Target | Checks actually run | Remaining limitation |
|---|---|---|
| macOS 26.5.2 arm64 | Archive install into a path containing spaces; setup; actual authenticated mail AI run; result/source presentation; backup/restore. Earlier MOD-01–06 cover separate roles, review and revocation. | No minimum-macOS guarantee, new company account or protected-document check. |
| Ubuntu 24.04 arm64, kernel 6.8.0-100-generic | Actual VM archive install/update; core commands; encrypted helper round trip; systemd 255.4 service; owner/peer checks; native sandbox; backup/restore. | No provider login was installed in the VM. The attempted AI job correctly remained waiting for authentication. |
| Linux amd64 on Ubuntu 24.04 | Packaged executable installed and run in an x86-64 emulated container; setup, module discovery, owner status, queued code input, backup/restore. | This verifies core execution under emulation, not a native amd64 driver/sandbox or service deployment. |

Windows, macOS amd64, other OS versions and kernels are not included in this
verified matrix. No actual EC2 instance, company VPN/VPC, managed-document
service or company credential was provisioned. The Linux host requirements and
the installation/update steps are in [SERVER.md](../setup/SERVER.md).

## Installed macOS walkthrough

The installer selected only the core by default. An intentionally wrong target
and an altered checksummed document were each refused before the installed
binary changed. The original package files were restored afterward.

The packaged `examples/mail/synthetic-day.json` produced completed job
`HBN27R6XON24GAOTM5MJQAHHNN`, attempt `Z4HTQUDKYGHNUZI3ZIJNHX4BJG`, in the fresh
validation data home. The actual driver ran for 57.5 seconds and observed only
`chunsu_mail.mail_source_get`. The preserved host validation accepted the
schema, pinned time, source coverage/mapping and acquisition-gap checks.
The local report and its source document were readable, and backup verification
and restoration into a separate home succeeded. These are operational checks;
no human usefulness judgment was invented.

### Preserved model-output failure

The reference-only `no-new-targets.json` input failed twice in the same installed
candidate: job `WCRJIVTH7DFUV67GYCIRGOLZ3S`, attempts
`L7CNVKACFKX7O5BGW2R6LGCYJ2` and `VEHXWC4ZVRZXPNEQUCTD4DPQXZ`. Both executions
retrieved the bounded sources but emitted business items supported only by
reference messages in `changes` mode. The host rejected both with
`changes mode cannot report a reference-only item as a new change`.

The pinned Skill explicitly prohibits this result. Its digest, source index
and schema matched the earlier successful MOD-03 no-new-target run. The evidence
therefore establishes inconsistent model compliance; it does not establish a
missing permission gate or justify weakening the validator. No third retry was
performed, no active Skill was changed, and neither `report` nor `publish` could
turn the failed job into an available report. Raw results and attempts remain
preserved for a selected review.

If this issue is prioritized for optimization, its failed attempts and the
earlier success form a concrete comparison cohort. A proposed instruction or
mechanism change must be compared with the other mail cases and pass the
owner's adoption criteria. This checkpoint does not claim the issue was fixed
or convert a host rejection into a successful AI outcome.

## Linux integration details

The final arm64 archive updated the earlier VM installation without resetting
its config or database. The helper's real Set/Get/Delete cycle succeeded with
a temporary non-production value and a separately provisioned private key.
A real systemd user service answered same-owner management requests and queued
bounded code-review input while paused. The immediate idle core measurement was
16,608 KiB RSS (about 16.2 MiB); it excludes the AI driver and is not a load test.

Owner enforcement rejected a different effective UID and a broadly readable
config. The live socket rejected a different peer UID even when that client
could bypass normal filesystem permissions. The same owner still received
status. The native driver sandbox ran under `NoNewPrivileges=yes`, read its
allowed package and could not read sibling data, the private key or the live
management socket. A controlled host listener was reachable outside the sandbox
but inaccessible to a tool process inside it. Namespace-private writes did not
alter the corresponding host file. See [MOD-04](MOD_04_SERVER_2026-09-11.md) and
[MOD-06](MOD_06_OWNER_ACCESS_2026-09-12.md) for prerequisites and earlier checks.

After service removal, access was revoked and a backup restored into another
home. The restoration preserved six audit receipts and the execution block,
kept admission paused and all route disclosure approvals false, and excluded
the credential key. No service startup link was left for this validation home.
The temporary executable-specific AppArmor rule and the task-only VM are removed
or stopped after the final archive verification; global security settings were
never disabled. Preserved task data remains available for inspection.

The first three-target candidate archives were approximately 13–14 MiB each;
the standalone core executables were approximately 21–23 MiB. Extra documentation
does not become a runtime dependency. Each final archive carries file checksums,
dependency versions/licenses and an unsigned local-candidate manifest.

## Validation and outstanding decisions

The full existing `go test ./...` and `go vet ./...` passed after the cross-cutting
owner/storage changes. Relevant existing tests/builds were also run with each
bounded implementation unit. Packaging Python and installer shell syntax were
checked. No test code was added. Public assets contain no runtime database,
account credentials or private model output. Nothing was pushed or published.

Human assessment of mail usefulness, golden expectations and candidate adoption
remains pending. Actual company deployment needs the selected host, account,
device/document permissions and allowed model route. A managed workstation is a
supported placement choice, not automatic proof of those permissions. Linux
authenticated AI, live Telegram and organization-wide SSO/tenant administration
are not claimed. Remote worker transport and mutating code execution remain
future modules around the single-writer core.

The completed technical units do not mark the original MS1–MS5 usefulness and
broader release milestones as accepted. The next deployment can use the built
candidate and runbook; its account and native-boundary checks must be performed
on that actual node.
