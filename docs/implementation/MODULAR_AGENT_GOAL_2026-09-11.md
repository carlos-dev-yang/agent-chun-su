# Modular agent goal — 2026-09-11

Status: active. Authorization: the user's eight annotations on the architecture
review and explicit request to pursue this goal autonomously. This extends the
implementation direction; it does not invent company credentials, a deployment
target, personal quality judgments, or permission to publish external changes.

## Accepted direction

1. Codex CLI/model pinning is a temporary stabilization measure. Keep the tested
   boundary until another configuration is verified, and separate agent drivers
   from the environment in which they execute.
2. Preserve results as work accumulates. Run AI reviews over explicitly selected
   results on demand; no compulsory grading after every task.
3. A reception agent discusses intent and available capabilities and submits
   bounded work. It has a separate context and authority from task executors.
   CLI and Telegram are transports, not owners of this orchestration.
4. Support an approved company cloud/server or dedicated managed workstation.
   The installed node uses the permitted user's/service identity. Actual device,
   document and company access is verified on that node when supplied.
5. Codex is the initial provider. Keep provider/model routing outside task
   semantics so local models or dedicated processing nodes can be connected
   later without changing the workgroup lifecycle.
6. Human ownership and versioning apply to all operating criteria: Skills,
   golden cases, rubrics, analyzers, optimization instructions and adoption
   policies. Reviewers propose findings; they cannot activate controls.
7. Keep Go + SQLite. One controller owns a data root and its database; workers
   receive bounded packages and return results. Additional workers must not
   share the SQLite file as independent writers.
8. Implement in the approved order below, recording actual evidence and bounded
   local commits. Local and Linux/server operation are required outcomes.

## Work units and completion evidence

| Unit | Relationship | Outcome | Evidence required | Status |
|---|---|---|---|---|
| MOD-00 | Planning | Reconcile the annotations, authority and implementation sequence | Linked goal record with explicit limitations | in_progress |
| MOD-01 | P3, P8 foundation | Versioned review criteria, selected-result review execution, explicit evaluation selection and adoption requirements | Focused existing checks; actual on-demand review; failed/unknown/missing checks cannot silently justify adoption; original jobs remain unchanged | ready |
| MOD-02 | P2, P6, chat | Separate reception orchestration and task admission from transports; generalize the workgroup boundary | CLI/Telegram share the reception host; separate task attempts; existing mail/Jira contracts continue to pass | not_started |
| MOD-03 | P2, P7 | Separate agent driver, execution environment and compatibility evidence | Temporary Codex pin remains enforced; role-specific isolated packages; unsupported combinations fail explicitly | not_started |
| MOD-04 | P7 | Portable installation and operation on macOS and Linux/server | Target binaries, server secret integration, service configuration, Linux installation/runtime evidence and recovery guidance | not_started |
| MOD-05 | P9 | A bounded development-work module, starting with read-only review | Explicit repository/source scope, preserved revision/diff, report and evidence through the common host; no implicit external writes | not_started |
| MOD-06 | Organization boundary | Node/workspace ownership and controlled access appropriate to the supported deployment | Owner-derived authority, isolated roots, protected management path, revocation/evidence; document unsupported shared multi-user modes | not_started |
| MOD-07 | Integration | Complete coherent local/server walkthrough and handoff | Existing regressions, installation/use instructions, actual vs pending environment checks, all technical units accounted for | not_started |

Dependencies follow the listed order. A small dependency extraction needed by an
earlier unit is permitted and recorded with that unit. Existing P3 human-usefulness
and adoption milestones are separate from technical implementation. No record may
pretend an AI-authored example is human-reviewed. Quality thresholds remain
human-configured rather than invented numeric defaults.

## Implementation boundaries

- Start as a modular monolith with separate agent processes. Add no database
  server, mandatory cloud service, public unauthenticated listener or broker.
- Host checks of access, input/output contracts and evidence are always applied.
  AI evaluation, analysis and optimization remain separate optional activities.
- Preserve legacy jobs and assets. Prefer additive, versioned file/record
  contracts; migrate the database only when the selected implementation needs it.
- The current data-root owner is the initial management identity. An external
  organization IdP and multi-user service contract need a concrete deployment
  selection; owner strings supplied by a model are never authentication.
- Run existing relevant tests. Do not create test code unless the user requests
  it. Manual CLI walkthroughs and synthetic data may verify new behavior, but
  synthetic success is not company access or personal acceptance.
- Runtime data, private material, model outputs and secrets stay outside Git.
  Package output remains ignored. Do not push, activate an external service or
  create paid cloud infrastructure without a concrete authorized target.

## Initial evidence

Baseline: `df20224`, clean checkout. Existing implementation has a restricted
report executor, immutable mail/Jira source gateways, independent imported
evaluations and CLI-owned natural chat. The 2026-09-10 review identifies the
missing quality/adoption and evaluator execution contracts. Docker/Colima are
installed locally; actual Linux runtime availability is still to be checked.

The user was asked for an optional existing SSH target. In its absence, Linux
validation will use a local Linux environment and the server runbook will retain
the unperformed EC2/company connection checks explicitly.
