# FLOW-01 — Durable conversation coordination and staged work

Status: design established; implementation and acceptance pending.

## Authorization and scope

On 2026-09-28 the owner requested this design and implementation, with the
primary agent owning design and GPT-6 Sol xhigh agents owning implementation
and one focused structural review. This explicitly authorizes implementation
delegation. Work remains in the current checkout with bounded local commits.
No production database, saved credentials, running service, remote branch or
deployment is changed by this development task. An additive SQLite migration
may be authored and exercised against disposable databases. Production
migration and activation require an explicit operational action.

The agreed scope covers public web research and existing mail, Jira and code
snapshot workflows. Normal conversation uses GPT-6 Sol medium. Collection
planning and mechanical extraction/normalization use GPT-6 Luna xhigh. Domain
judgment, report synthesis and optional result review use GPT-6 Sol xhigh.
Astra is not a default or an automatic fallback. Code judgment never goes to
Luna. Existing independent evaluation/adoption remains optional and human-owned.

## 1. Ownership and process boundaries

There is one durable logical coordinator per conversation, not one permanent
model process. A conversation can reference multiple jobs. Every model call is
an ephemeral, bounded process constructed from persisted conversation context
or a pinned task package. The Telegram receiver and local CLI are transports.

The existing Go controller remains the only SQLite writer and task scheduler.
It owns jobs, step eligibility, attempts, cancellation, artifact publication and
result events. Model output proposes bounded actions; it never edits queue
state, chooses filesystem paths or grants itself permissions. The initial
execution limit stays one active task process. Logical steps do not require
separate services, a message broker or a general workflow engine.

The front end must remain usable without a healthy controller. Conversation
state therefore uses a private file-backed store with its own lock, separate
from the controller's long-held lock. Queue mutation continues through the
existing private control endpoint. Result handoff uses durable task events and
idempotent conversation ingestion; front-end processes never write task SQLite.

## 2. Durable conversation contract

Persist a host-generated conversation ID, channel/owner binding, a revision,
ordinary user/assistant messages, host event references, linked job IDs,
pending decisions and processed result-event IDs. The active conversation
pointer belongs to a channel/owner binding, never to a transient receiver PID.
CLI supports explicit resume/select and reports its conversation ID; paired
Telegram restores its active conversation automatically after restart.

Only ordinary admitted conversation messages are retained. Credential input,
authentication callbacks, arbitrary provider diagnostics and unbounded raw
source/tool bodies are excluded. Files use the existing private-directory,
private-file, no-symlink and atomic-write helpers. This intentionally supersedes
the old no-transcript-retention rule for ordinary dialogue; document that change
in setup/help. Retention is explicit/manual in this release. Reset archives the
current conversation and opens a new one; it does not delete or cancel jobs.

Generation consumes recent dialogue, durable task references and explicit
decisions within configured limits. Persisted history is not silently dropped.
If context cannot fit, give an actionable context-limit result while preserving
the conversation and jobs. Automatic lossy summarization is not required for
the initial release. Restart or uncertain delivery never replays a user action.
Record interrupted turns as uncertain until reconciled. Per-conversation
revision checks reject a reply based on superseded user instructions.

## 3. Job, task step and attempt

A job is the durable user objective. It carries the original request, origin
conversation/message reference, request revision, completion criteria, source
scope, configured budget, intended destination and a workflow version.
Existing job IDs and artifacts remain readable. Destination is intent, not
authorization: supported initial delivery is a local artifact or the paired
originating conversation. No new email/Jira write connector is introduced.

A task step is a durable unit inside that job. Store a stable step ID, stage,
ordinal/dependencies, pinned input artifact IDs/digests, expected output schema,
executor model/effort/identity, instruction version, state, gate policy and
current attempt. Each execution/retry gets a distinct attempt. A retry is not
the next stage. Preserve failed outputs and reject stale-attempt publication.

Use additive tables/fields for steps and their attempts, keeping existing
job/attempt semantics and historical runs intact. New staged jobs must have
actual step records driving execution, not a decorative list around one opaque
AI call. Legacy single-stage jobs remain inspectable/recoverable through their
existing path. Clearly version any new package and execution contract.

Step states: pending, ready, running, waiting_input, waiting_auth, retry_wait,
completed, failed, cancelled, superseded. Dependencies become eligible only
after successful validated publication; a failed or partial prerequisite is
never silently treated as complete. Partial evidence can proceed only when its
declared schema and existing domain completion policy explicitly permit it.

The parent aggregates progress and becomes complete only after required steps
and delivery bookkeeping finish. A failed delivery can be retried from the
validated result without rerunning collection or synthesis. Cancellation stops
the active process and blocks future steps. Recovery reconciles process
identities before offering resume; unknown execution is never auto-replayed.

## 4. Stage boundaries

| Stage | Executor | Input and output |
|---|---|---|
| Intake | Sol medium | Capture the user's objective, supported scope and destination; register a job through host validation. |
| Collect | Go connectors, Luna xhigh where model selection is needed | Public web uses bounded search/open actions; saved mail/Jira/code use existing authorized snapshots and scoped lookup. Publish immutable raw evidence and acquisition gaps. |
| Refine | Luna xhigh | Extract source-backed facts, exact relevant excerpts and structured fields. Publish a validated evidence bundle. No priority, defect or business judgment. |
| Synthesize | Sol xhigh | Original objective, domain Skill, pinned metadata and refined evidence produce the existing domain report contract or a web answer with citations. |
| Validate | Go | Validate schemas, input provenance, source identities/coverage, permitted tools, known gaps and stale execution identity. This is not mandatory AI review. |
| Deliver | Go/front end | Publish the validated artifact and a durable completion event to its allowed destination. Track generated, available, delivered and unconfirmed separately. |

Collection and refinement have distinct outputs/checkpoints even where source
acquisition is deterministic Go code. Domain adapters own their exact facts and
coverage rules. Do not force mail/Jira/code semantics into a lossy generic text
summary. Preserve pinned Jira fields/citation IDs and mail target/reference
dispositions. Code extraction preserves file/line provenance for Sol judgment.

A normalized evidence envelope includes schema version, job/step/request
revision, input digests, source ID/URI, source timestamps, extracted facts,
supporting excerpts, omissions/truncation/unavailability and artifact references.
Every normalized source must map to admitted raw evidence; invented IDs and
missing required sources fail validation. Excerpts must be grounded in source
content where mechanically checkable. Schema success does not prove semantic
truth. Source text and child output remain untrusted evidence.

Sol receives the original objective and enough verified metadata/excerpts to
reason from the evidence. It does not silently gain raw-source/network access.
An explicit insufficient-evidence result can create an additional bounded
collection/refinement sequence through the controller, retaining ancestry and
budget accounting. Scope expansion or a budget increase requires user input.
Do not permit an unbounded self-created task loop.

## 5. Conversation handoff and user control

Persist a result event with a stable ID, originating conversation ID, job and
step IDs, request revision, result artifact ID/digest, bounded summary, source
references, gaps, status and required user decision. Publish only after the
referenced artifact is durable and validated. The coordinator validates access
and artifact integrity before including it as a host event in a new generation.
It can retrieve bounded details through a host-owned result reader.

When a task completes, the receiver consumes the event while idle or queues it
behind the current conversation turn. It must not interrupt user input or run
two parent generations concurrently. Ingestion is idempotent across restart;
delivery uses durable receipts. Unknown Telegram send results stay unconfirmed
rather than being blindly resent. A reset/archived conversation retains its
job links; old results are not injected into an unrelated active conversation.
Explicit job selection/resume makes them available in a new conversation.

Expose inspect/progress/results, pause before the next step, resume, cancel,
retry a failed step and resolve a pending decision through bounded CLI/chat
commands. Default auto-advance operates within the admitted scope. A manual
checkpoint mode lets the owner inspect collected/refined evidence before the
next stage. Pause must not rewrite a completed artifact; revisions create new
artifacts and supersede dependent steps. Reconcile any running process before
applying a revision that invalidates its output.

Chat remains available while a child task executes. New user instructions
update the job only via an explicit, validated action with the current request
revision. A conversational mention alone must not mutate unrelated running work.

## 6. Model policy and compatibility

Add reasoning effort to the executor identity/configuration, and resolve model
and effort centrally per role. Defaults: reception=Sol/medium,
collection/refinement=Luna/xhigh, synthesis/task/review=Sol/xhigh. Keep explicit
legacy settings inspectable; offer an explicit policy-application path for an
existing home, preserving executable/environment choices while invalidating
disclosure grants affected by model/effort/route changes. Installer defaults
must use the new policy, and never silently select Astra after a failure.

The new model names are supported by official OpenAI model documentation, but
local CLI compatibility must still be checked. Do not label an untested model/
version pair validated or widen the executable version gate without evidence.
Use disposable synthetic inputs for actual model/boundary checks where the
installed authenticated CLI permits them. Report compatibility blocks honestly.
No automatic CLI installation or production runtime reconfiguration is implied.

Source access authorization applies to every model receiving private evidence,
including the coordinator if report content is handed back. Do not copy a legacy
single-model disclosure grant to newly introduced models. Pin both stage routes
and check revocation on execution and handoff. Keep synthetic validation paths
usable without live account access. Existing source-gateway constraints remain
enforced even as stage identities become explicit.

## 7. Delivery units and acceptance

| Unit | Deliverable | Required evidence | Status |
|---|---|---|---|
| FLOW-00 | This design and implementation-plan entry | Code-grounded ownership and contracts | designed |
| FLOW-01 | Role model/effort policy and durable conversation store | Configuration projection; restore/reset; compatibility outcomes | pending |
| FLOW-02 | Durable job steps, attempts, controls and recovery | Dependency gating; stale rejection; cancellation/restart; migration on disposable schema-4 DB | pending |
| FLOW-03 | Mail/Jira/code staged extraction and synthesis | Preserved source validation; stage-specific executor receipts; targeted existing checks | pending |
| FLOW-04 | Web jobs and conversation result handoff | Public boundaries/quota; job admission; durable event ingestion and restart deduplication | pending |
| FLOW-05 | Integration, docs and focused structural review | Coherent CLI/Telegram walkthroughs; one structural review; explicit unperformed live checks | pending |

Implementation can adjust package/file layout without changing these contracts.
New architecture, model substitutions, live permission changes or deployment
decisions must be reported rather than silently inferred. Do not introduce test
source files: use existing tests and disposable manual/synthetic walkthroughs.
Batch validation by changed integration boundaries; cross-cutting checks are
appropriate once the complete change is integrated. Review once for (1) durable
state/recovery, (2) source and model permissions/provenance, (3) conversation
handoff/concurrency. Follow up only on concrete findings and affected fixes.

Preserve the self-updater's migration refusal. Document that this schema change
requires an explicitly managed upgrade with backup and rollback planning;
do not present a normal `/update` as sufficient production activation.
