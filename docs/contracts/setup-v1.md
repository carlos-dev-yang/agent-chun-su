# Local setup host v1

Scope approved on 2026-09-11: guided setup conversation and host processing,
preserving the existing report executor boundary. No DB schema change.

## Entry points and ownership

`chat --guided [REQUEST]` is a fixed terminal dialogue, not an LLM session.
The default `chat` uses the separate [AI chat contract](chat-v1.md). Service IDs and
aliases come from the embedded catalog. It accepts one service at a time, handles
Gmail questions, and requests fixed host actions. Unknown input is not executed.

The client connects to the existing private management socket. When no owner
exists it starts the same executable as `setup serve`, and stops that child when
the conversation ends. It never stops a pre-existing worker. Setup-only service
owns the normal controller lock but runs no queue or schedules. A worker can
serve setup requests while running reports. Authentication runs asynchronously;
the client polls results and can request cancellation.

The management channel is a trusted local human-control surface with the existing
private filesystem/socket permissions. It is not an additional same-user sandbox.
Report executors receive neither setup operations nor access to this socket;
their existing allowed tools, permissions, Skill discovery and packages are unchanged.

## Fixed operations

Requests use the existing `control.Request` envelope with operation `setup.*`
and a typed `input` object. Unknown operations/fields and invalid task IDs fail.
No request accepts shell text, an arbitrary package installer, credentials, URLs
to execute, permission changes, rule adoption, or scheduler activation.

| Operation | Input | Result/effect |
|---|---|---|
| `setup.prepare` | catalog service ID | Install embedded public setup assets under `setup-skills/<digest>/connect-services`; return manual path/hash |
| `setup.gmail.start` | fresh task ID, local absolute client JSON path, exact account, explicit Gmail policy | Async existing Desktop OAuth/PKCE/Keychain flow; save new binding after identity/scope validation |
| `setup.gmail.check` | fresh task ID, existing connection ID | Refresh/profile check within its current permission, with no message-body retrieval |
| `setup.status` | task ID | Return current status; interrupted persisted work is not restarted |
| `setup.cancel` | task ID | Cancel pending work; a completed connection remains completed |
| `setup.stop` | empty | Stop standalone `setup serve`; a report worker rejects this operation |

One authentication/check runs at a time per controller. A duplicate task ID is
rejected; the client must inspect its status instead of automatically restarting.
New OAuth creates a new binding; in-place reauthorization remains the existing
standalone CLI operation. No setup action admits a report or enables live AI disclosure.

## Evidence and recovery

Tasks persist under `state/setup-tasks/<id>.json` using private atomic file writes.
States: `running`, `awaiting_auth`, `connected`, `failed`, `cancelled`; an incomplete
record outside its running host is presented as `interrupted`. Persisted records
omit the OAuth URL, client file contents/path, account input, query and tokens.
The live OAuth URL is memory-only and sent over the private channel to the local
terminal client. `setup status` strips it from diagnostic output.

The dialogue itself is not logged. Terminal echo/scrollback is still local user
output; users must not paste secrets. Auth uses the existing provider browser and
Keychain paths. All network and Keychain cleanup follows existing Gmail logic.

`connected` means an account/permission check succeeded. It does not establish
message retrieval, rule quality, golden evaluation, or release approval. A crash
between provider completion and final status recording requires checking the
connection list before attempting again; no automatic retry infers non-completion.

Setup assets are content-addressed from a sorted file/hash manifest. Same bytes
are reusable; conflicting local files are preserved and produce an error. This
is a content digest, not a publisher signature or release attestation. Setup
assets are reconstructible from the binary; setup tasks are diagnostic records,
not report evidence or completed golden runs. Existing backup coverage is not
expanded by these additional files.
