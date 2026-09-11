# Natural chat v1

Approved on 2026-09-11: converse freely through the configured AI executor and
restrict executable actions in the host. This adds a terminal conversation client;
this terminal contract adds no public chat server or DB migration. The subsequent
[Telegram transport](telegram-v1.md) projects a smaller capability set into paired private DMs.

## Execution path

The MOD-02 implementation moves intake history, action iteration, channel
capabilities, validation, dispatch and audit to `internal/reception`. CLI and
Telegram provide user input/output and cancellation. Local Gmail interaction
is a callback, not a separate conversation engine. `internal/conversation`
remains the typed message/Skill contract.

`chat [REQUEST]` uses the selected `executor.kind/path/model` and existing Codex
CLI login. `chat --guided` retains local setup without AI. The supported adapter
pins the same tested CLI/model as report execution; other versions fail with a
configuration message instead of quietly changing the boundary.

Each generation is an ephemeral Codex process with the embedded `converse` Skill,
a host-generated response schema, capability list and in-memory conversation
history. Native shell, web, MCP, plugin, skill discovery, repository instructions
and other tool features are disabled. The existing minimal/read-only filesystem
profile exposes the current turn's public Skill/schema directory, excluding
management state, report inputs, golden answers and credentials from tool access.
Codex authentication/provider traffic is still required; disabling tool networking
does not mean the AI request runs offline. Report work runs in a separate job
attempt with its own source gateway, Skill and result contract.

The model returns `{message, action: {name, service, reference}}`. Strict parsing
rejects unknown fields, actions, services, unexpected parameters and invalid IDs.
The client validates again before host dispatch. Text never becomes a shell command.
Unexpected native tool items terminate the generation without dispatch. Item-level
CLI diagnostic warnings grant no tool; success still requires a completed turn,
valid reply, successful exit and confirmed process-group cleanup.

## Capabilities

The list and response schema derive from `internal/conversation.Capabilities`.
Adding a manual/catalog entry expands guidance only; adding a real action requires
an explicit host handler and validation. Model requests cannot change this list.

| Action | Host behavior |
|---|---|
| `none` | Reply, draft, explain, ask questions; no host effect |
| `runtime_status` | Read selected controller status fields through existing private control channel |
| `read_guide` | Read the selected public embedded service manual into dialogue context |
| `install_guide` | Call `setup.prepare`; install content-addressed public setup pack locally |
| `gmail_setup` | Install Gmail guide; hand off to local existing/new connection questions and async auth/check |
| `list_jobs` | Read bounded existing job ID/workgroup/status/time summaries; omit requests, source bodies and reports |
| `show_report` | Display a hash-verified saved report directly to the local terminal; use a listed or user-specified ID |
| `delegate_job` | Submit the same verified saved input as a separate task at the user's request; use a listed or user-specified ID and current active controls |

Supported user-requested reads/local guide installation need no repeated approval.
New Gmail account scope and authentication remain explicit local steps. Unsupported
requests should produce useful work that is available now (such as a message draft)
and explain the missing connector/capability. Remote writes, arbitrary installation,
arbitrary new source admission, rule adoption, evaluator changes and schedule activation are
not chat capabilities. Local guide installation is not external account connection.

Delegation accepts a saved job ID, not a filesystem path, account credential,
candidate-control digest or executable command. Host admission validates the
module input and preserves `delegated_from` with the bounded user request. The
reception AI receives only the new job's ID/workgroup/state. A paused queue or
setup-only host can accept a job without executing it; `queued` must not be
reported as completion. Reprocessing saved input never advances collection
coverage. Each input can be delegated once per conversational turn. Interrupted
or uncertain admission is inspected, not automatically retried.

The client reuses an existing controller or starts/stops its own setup-only child,
as in [setup v1](setup-v1.md). The host is a trusted local control surface, not an
additional same-user sandbox. The model process does not receive its control socket.

## Dialogue, interruption and evidence

Host results are separate history events. After each successful action, the model
continues the user's request using its actual result. The UI labels host outcomes
separately from AI prose; an AI assertion alone is not completion evidence. A failed
host action is displayed locally and not automatically retried. At most the smaller
of the named chat action limit and configured tool-call limit executes per request.

New terminal input cancels and reaps the pending generation before processing the
new message. `취소` cancels the current generation/local setup; `종료`, EOF or Ctrl-C
ends the conversation. `/새대화` clears in-memory history and known report IDs.
History exceeding the configured source-byte limit requires reset; old instructions
are not silently dropped. A caller should keep stdin open while awaiting a reply;
piping a request then immediately closing stdin is cancellation, not batch mode.

Chat text and returned job summaries are sent to the configured AI provider.
Account/client-path/auth-URL/token inputs entered in the local Gmail handoff are not
added to model history. Local errors are shown only on the terminal; the model gets
a bounded failure summary. Report text is printed directly to the user; the model
receives only display status and must not claim to have read or summarized that body.

`chat/<session>/<turn>/` holds the public Skill/schema, process identity, executor
version/model/usage/outcome and arguments/prompt/schema/Skill/reply digests. `action.json` records
intent before dispatch and status/result digest after dispatch. The app does not
persist transcript, reply text, raw provider diagnostics or auth values. This is
operational metadata, not semantic evaluation or a release attestation. Provider
retention and the user's local terminal scrollback are outside that claim.

There is no automatic replay of an interrupted action. A crash after host execution
but before final metadata can leave an uncertain intent; inspect the actual setup
or job state before repeating it. Orphan detection stops the client; hard-crash
recovery of chat processes and automatic reconciliation are not implemented.
Existing report backups/retention do not expand to chat metadata in this version.
