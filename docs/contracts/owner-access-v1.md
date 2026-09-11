# Owner access and revocation v1

A data home is owned by one OS user. macOS/Linux checks require that the root,
state directory, configuration, database and controller lock have the expected
owner and private permissions. Read-only opens enforce the same ownership.
Private credential keys also require that owner; a helper executable must belong
to that user or the administrator and must not be writable by other users.

Management uses a private Unix socket. Both client and server verify the peer's
OS credentials (`LOCAL_PEERCRED` on macOS, `SO_PEERCRED` on Linux) against their
effective user ID. Request JSON cannot supply a trusted identity. The native AI
boundary keeps that socket and the control assets outside the agent's scope.
Use authenticated SSH as the owner for remote operation; no public management
listener is added. Separate organizations/users need separate OS accounts and
data homes, not a shared writable SQLite database.

`access status` shows the observed owner, execution block and per-role source
disclosure flags. `access revoke` persists an execution block, cancels the
controller's active task, pauses queued/scheduled work and clears mail/Jira/code
disclosure approvals on every configured route. Agent drivers check the block
before new calls and watch it during running task/reception/review calls at the
configured poll interval. Native process cleanup and saved interruption evidence
remain in the driver. Data already disclosed to a provider cannot be recalled.

`access resume` explicitly removes the block. It does not reapprove source
disclosure, unpause the queue, reconnect accounts or restore provider credentials.
Review those decisions individually. Provider OAuth revocation and connector
disconnect remain their respective explicit operations. A standalone foreground
review owns the writer lock; stop it with Ctrl-C before issuing another local
writer command. Existing controller-managed sessions accept the access command
through their management socket.

Host receipts under `state/audit` record OS user/process, time, action, record ID
and digest, without copying credential values or arbitrary payloads. A
`record.prepared`/`configuration.prepared` receipt proves preparation, not a
successful later commit. The corresponding stored record/current active pointer
remains authoritative. Successful control selections have their own receipt.
Use `access history --limit N` to inspect recent receipts. Backups include them
and preserve an execution block; restored historical user IDs are history, not
authorization on the new node.

These are local owner records, not independent attestation that a named human
was physically present. A trusted OS owner can edit local files; administrators
and a compromised host remain outside this boundary. Central IdP, shared-account
multi-user roles, external tamper-resistant audit, device compliance and protected
document access require the actual organization's deployment configuration.
The application does not bypass that organization's device or document policy.

Run retention removes that run's content only. Reviewer packages/results,
evaluation/proposal records, audit, acquisitions, backups and external copies have
separate retention needs and are explicitly listed as remaining copies. Do not
treat the run-retention command as an organization-wide erasure operation.
