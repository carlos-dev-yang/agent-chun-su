# Chun-su Implementation Plan

**Status:** Proposed implementation roadmap  
**Date:** 2026-09-05  
**Scope:** Core control framework first; optional platform capabilities later

## 1. Purpose

Chun-su is a human-controlled framework for delegating work to replaceable AI
executors without delegating ownership of the operating rules.

The executor may reason, use permitted tools, and produce artifacts. It does not
own the rules, release criteria, retry policy, evaluation datasets, scores, or
the decision to promote a proposed improvement. Those controls remain outside
the execution environment and remain human-owned.

The first implementation should prove this control model with the smallest
durable system that can:

1. accept a job;
2. construct an immutable execution package from human-authored controls;
3. run one replaceable executor in an isolated container;
4. collect its output as immutable evidence;
5. apply deterministic gates and a human-authored release policy;
6. complete, retry, or escalate the job without relying on executor self-report;
7. evaluate completed work separately with a versioned golden set; and
8. preserve evidence from which a human can improve the controls.

This document narrows the implementation order described in `DESIGN.md`. It is
not a commitment to build every possible agent-platform feature.

## 2. Product Thesis

### 2.1 The executor is a worker, not the authority

An executor is a replaceable implementation of one interface. A future model,
agent framework, local CLI, or managed runtime must be swappable without
changing job lifecycle, gate semantics, evidence, or evaluation history.

### 2.2 Human ownership does not mean approval before every run

The human owns the criteria. The framework enforces them automatically.

For a normal low-risk workgroup, a job that passes its deterministic gates can
complete automatically. Human attention is required only when the configured
policy requires it, such as:

- admission is invalid or ambiguous;
- deterministic gates keep failing after the retry budget is exhausted;
- the workgroup is explicitly configured for manual release;
- the requested action has an external or irreversible side effect; or
- no declared policy can safely decide the next transition.

### 2.3 Release and evaluation are separate loops

Release gates answer: **Is this artifact allowed to complete or be published?**

Evaluation answers: **How good was the result, and what evidence should inform
the next human-authored control version?**

Golden-set scores must not silently become per-job release gates. Operational
feedback must not silently become golden truth. AI-generated improvements must
not silently become active controls.

### 2.4 The framework must outlive any execution attempt

The controller runs outside executor containers and owns durable state. A
container may crash, time out, disappear, or return malformed output without
losing the authoritative job state.

The initial target is a single always-on node, such as a Mac mini or a Linux
server. Distributed orchestration is deliberately deferred.

## 3. Target Architecture

```text
                         HUMAN-OWNED CONTROL
              rules · schemas · capabilities · gates
                retry policy · release policy · golden
                                  |
                                  v
+------------------------- Persistent Controller --------------------------+
| admission · packaging · queue/lease · recovery · retry · release decision |
|                         durable job state                                 |
+----------------------+--------------------------+-------------------------+
                       |                          |
                       v                          v
              Immutable WorkPackage       Immutable Event Ledger
                       |                          |
                       v                          |
              ExecutorAdapter                    |
                       |                          |
                       v                          |
             +-- isolated container --+          |
             | replaceable AI executor |          |
             | read package / write out|          |
             +------------+------------+          |
                          |                       |
                          v                       |
                 Immutable ArtifactBundle -------+
                          |
                          v
               Deterministic Gates + Release Policy
                          |
                +---------+----------+
                |                    |
             complete          retry / human
                |
                v
       asynchronous evaluation and audit
                |
                v
      scores · feedback · change proposals
                |
                v
          human review and promotion
```

The system has three planes:

| Plane | Responsibility | Authoritative owner |
|---|---|---|
| Control plane | Policies, lifecycle, gates, release, retry, promotion | Human-authored definitions enforced by the controller |
| Execution plane | Reasoning, permitted tool use, artifact production | Replaceable executor inside an isolated attempt |
| Evidence plane | Events, artifacts, scores, audits, feedback, proposals | Controller writes; human interprets and promotes |

AgentCore or another managed platform may later implement parts of the
execution plane. It must not become the owner of Chun-su's control semantics.

## 4. Authority and Storage Boundaries

Directory names alone are not a security boundary. The runtime must enforce
separate permissions.

| Area | Contents | Write authority |
|---|---|---|
| `control/` | Workgroup rules, schemas, gates, capabilities, retry and release policies, evaluation definitions | Human-controlled workflow only |
| `state/` | Jobs, attempts, leases, transitions, active policy references | Controller only |
| `artifacts/` | Work packages, executor output bundles, manifests | Controller only after collection |
| `evidence/` | Append-only run events, gate verdicts, scores, audits, feedback | Controller and designated evaluators |
| `proposals/` | Candidate rule, schema, skill, and golden changes | AI or automation may write; never authoritative |

There must be no code path from an executor, evaluator, optimizer, or proposal
generator that directly modifies `control/` or promotes a control version.

The first implementation may store these areas on one machine, but it must keep
their interfaces and permissions distinct so that storage can later move to a
database, object store, or managed service independently.

## 5. Core Contracts

### 5.1 JobRequest

`JobRequest` replaces the ambiguous user-authored “Card.” It is a minimal
request envelope, not a second policy document.

Required fields:

- `job_id`
- `workgroup`
- `instruction`
- `input_ref` or materialized input
- `requester`
- `submitted_at`

Optional request-level constraints are allowed only when the workgroup schema
defines them. Free-form `must` statements are excluded because they cannot be
reliably connected to a gate or capability restriction. A repeated constraint
belongs in the workgroup's versioned control definition.

### 5.2 WorkPackage

An immutable snapshot constructed by the controller. It contains:

- the normalized `JobRequest`;
- materialized input;
- a pinned `policy_version`;
- rules and skills needed by the executor;
- the output schema;
- the capability grant and tool definitions;
- gate, retry, release, and budget settings;
- a canonical manifest; and
- `pack_digest`.

The executor sees only this package and its writable output directory.

### 5.3 Attempt

One isolated invocation of an executor against one `WorkPackage`. A retry is a
new attempt and a new container. The controller, not the executor, creates and
numbers attempts.

### 5.4 ArtifactBundle

An immutable snapshot of collected executor output plus a manifest and
`artifact_digest`. Runtime metadata such as timestamps, logs, token counts, and
exit status is stored beside the bundle and is not mixed into its content
digest.

### 5.5 GateVerdict

A deterministic result produced from a pinned `WorkPackage` and an immutable
`ArtifactBundle`:

```text
PASS
FAIL <gate_id> <stable_reason_code> <bounded_details>
```

An LLM judgment is not a deterministic gate. If used later, it belongs to the
evaluation plane unless a human explicitly defines a different release policy.

### 5.6 ReleaseDecision

The controller combines the gate verdict with the pinned release and retry
policies to choose one transition:

- `COMPLETE`
- `RETRY`
- `NEEDS_HUMAN`
- `FAIL`
- `CANCEL`

Supported release modes should be explicit per workgroup:

- `auto_on_pass` — default for safe, reversible work;
- `manual_on_pass` — for sensitive or externally visible work; and
- `draft_only` — produce an artifact but never perform the external action.

Passing a gate does not inherently require human approval. Likewise, a failed
gate cannot be overridden by the executor.

### 5.7 RunEvent

An append-only record of every authoritative lifecycle transition. Each event
contains the job, attempt, prior state, next state, policy version, relevant
digests, reason code, actor, and timestamp.

### 5.8 EvaluationResult and ChangeProposal

An `EvaluationResult` links a dataset version, evaluator version, policy
version, executor identity, input case, artifact digest, and score.

A `ChangeProposal` may suggest changes to controls, but only a human-controlled
promotion process can make those changes active.

## 6. State Model

```text
RECEIVED -> ADMITTED -> PACKAGED -> QUEUED -> RUNNING -> COLLECTED
                                                           |
                                                           v
                                                        VERIFIED
                                                           |
                            +------------------------------+----------------+
                            |                              |                |
                            v                              v                v
                        COMPLETED                    RETRY_QUEUED      NEEDS_HUMAN
                                                          |                |
                                                          +-> QUEUED       +-> COMPLETED
                                                                           +-> FAILED

Any non-terminal state may also transition to CANCELLED under controller policy.
Invalid admission terminates as REJECTED. An unrecoverable execution or exhausted
retry budget terminates as FAILED or NEEDS_HUMAN according to policy.
```

Only the controller writes authoritative state. Executor status is observed
input, not state truth.

## 7. Simplified Integrity Model

The core requires only three integrity identifiers:

1. `policy_version` — immutable version of the human-owned control set;
2. `pack_digest` — digest of the canonical WorkPackage manifest and contents;
3. `artifact_digest` — digest of the immutable ArtifactBundle contents.

Do not promise “the same commit plus the same card always has the same hash.”
External input may legitimately change. Instead, identical canonical package
content must have the same `pack_digest`.

Canonical digests exclude envelope and runtime metadata such as job ID,
requester, submission time, attempt start time, runtime ID, logs, and token
usage. The resolved instruction, materialized input, control files, capability
grant, schemas, and policies remain digest inputs. Excluded values remain in
event metadata.

Gates validate the pinned package and its digest. They do not compare the
package to the repository's current `HEAD`, because controls may legitimately
change while an older job is still running.

## 8. Executor Port

The controller depends on one asynchronous interface:

```text
submit(work_package_ref) -> execution_ref
inspect(execution_ref) -> queued | running | completed | failed | lost
collect(execution_ref) -> artifact_bundle_ref
cancel(execution_ref) -> acknowledgement
```

The initial adapter uses a local OCI-compatible container runtime. Later
adapters may use another local agent, a team server, or AgentCore Runtime.

Every adapter must preserve the same boundaries:

- a fresh isolated environment per attempt;
- read-only WorkPackage;
- one writable output location;
- no access to `control/`, golden data, other jobs, host credentials, or
  unrelated host files;
- declared CPU, memory, time, and output-size limits;
- no network by default;
- network and tools granted through explicit capabilities; and
- no release credentials inside the execution environment.

## 9. Evaluation Data Model

The following evidence paths must remain separate:

| Dataset or signal | Purpose | May block a live job? | Promotion authority |
|---|---|---|---|
| Golden set | Curated offline regression of expected behavior | No | Human |
| Benchmark suite | Compare executors, policies, cost, latency, and robustness | No | Human |
| Production audit sample | Review representative completed jobs | No, unless a separate incident policy acts later | Human |
| Operational feedback | Record complaints, failures, ambiguity, and incidents | No | Human classifies it first |
| Gate verdicts | Enforce deterministic minimum requirements for a specific job | Yes | Human-authored gate policy |

An operational failure may become a golden case only through a deliberate human
promotion step. Golden-set changes are versioned independently from rules and
executor versions so that score changes remain explainable.

## 10. Phased Delivery Plan

| Phase | Outcome | Core or extension |
|---|---|---|
| 0 | Stable contracts and authority boundaries | Core |
| 1 | Durable, always-on control kernel | Core |
| 2 | One isolated end-to-end local workflow | Core |
| 3 | Golden evaluation and human promotion loop | Core complete |
| 4 | AgentCore as an optional executor provider | Extension |
| 5 | Tools, external release, Noel, team operation, advanced optimization | On demand |

### Phase 0 — Freeze the Core Contracts

#### Objective

Remove semantic ambiguity before implementation and establish the seams that
allow later capabilities to be added without changing the kernel.

#### Deliverables

- Final definitions for `JobRequest`, `WorkPackage`, `Attempt`,
  `ArtifactBundle`, `GateVerdict`, `ReleaseDecision`, `RunEvent`,
  `EvaluationResult`, and `ChangeProposal`.
- JSON Schemas for persistent data contracts.
- The state transition table with one authoritative owner for every transition.
- A minimal `ExecutorAdapter` contract.
- A workgroup control layout containing only:
  - rules and executor context;
  - input and output schemas;
  - capability grants;
  - deterministic gates;
  - retry, release, and budget policies; and
  - evaluation references.
- The three-digest integrity model.
- Architecture decisions for the initial implementation:
  - one long-running controller process;
  - SQLite as the durable single-node state store;
  - filesystem-backed immutable packages, artifacts, and evidence;
  - JSON for machine contracts and YAML for human-authored policy;
  - an OCI-compatible container runtime for execution; and
  - no message broker or distributed workflow engine.

#### Explicit removals from the current design

- Replace “Card” with `JobRequest`.
- Remove free-form per-job `must` rules.
- Rename pre-run “approval” to deterministic admission; it is not routine human
  approval.
- Remove the rule that every retry-then-pass result must be manually held.
  Human review is driven by the workgroup release policy or exhausted failure.
- Remove live-run comparison against unrelated golden cases.
- Remove current-`HEAD` comparison from artifact gates.
- Replace the complex pack-hash promise with canonical content digests.

#### Deferred

All execution, UI, AgentCore integration, external tools, multi-user access,
LLM judges, optimization, distributed workers, and Noel knowledge integration.

#### Exit criteria

- Every contract has a producer, consumer, owner, and version rule.
- Every state transition has one controller action and one stable reason code.
- Replacing an executor requires only a new `ExecutorAdapter`.
- No executor-facing contract grants authority to modify or promote controls.

### Phase 1 — Build the Persistent Control Kernel

#### Objective

Create the always-on framework that owns job lifecycle independently from any
executor process.

#### Deliverables

- A single controller service with a durable work loop.
- SQLite repositories for jobs, attempts, leases, transitions, and policy pins.
- An append-only event ledger.
- Admission validation and immutable WorkPackage construction.
- Queue claiming with leases rather than in-memory ownership.
- Attempt timeouts, cancellation, and bounded retry decisions.
- Startup reconciliation for jobs left in `QUEUED`, `RUNNING`, or `COLLECTED`.
- Immutable filesystem stores for packages and artifact bundles.
- Minimal commands or local API for:
  - submit;
  - inspect status and history;
  - cancel;
  - resolve `NEEDS_HUMAN`; and
  - inspect the exact policy version and digests used.
- Interfaces, but not implementations, for:
  - `ExecutorAdapter`;
  - `ArtifactStore`;
  - `Gate`;
  - `ReleaseAdapter`;
  - `EvaluationAdapter`;
  - `ToolGateway`;
  - `KnowledgeProvider`; and
  - `Notifier`.

The controller may use a non-AI placeholder executor during this phase so that
the lifecycle is implemented before provider-specific behavior is introduced.

#### Deferred

Multiple controller nodes, message queues, web UI, remote users, rich
notifications, and production release side effects.

#### Exit criteria

- Restarting the service does not lose or duplicate a job.
- A lost attempt is detected and reconciled after its lease expires.
- A job reaches exactly one terminal state unless a recorded human resolution
  moves it out of `NEEDS_HUMAN`.
- The controller can reconstruct a job's lifecycle from events and digests.
- A job remains pinned to its original policy version after controls change.

### Phase 2 — Complete One Isolated Local Workflow

#### Objective

Prove the full control boundary using one real executor and one low-risk,
read-only workgroup.

#### Deliverables

- One local OCI container adapter.
- One LLM executor implementation behind the adapter.
- Fresh container creation for every attempt.
- Read-only package mount and writable output mount.
- Network disabled unless the workgroup explicitly requires an allowlisted
  model or tool endpoint.
- Host-side collection that freezes output before any gate reads it.
- Initial deterministic gate types only:
  - schema validation;
  - completeness;
  - safe path and file checks;
  - package and artifact integrity;
  - bounded workgroup-specific invariants; and
  - time, output-size, and budget checks.
- Controller-owned retry with a new container for each attempt.
- Structured, bounded failure feedback for retry; the executor cannot choose
  whether it receives another attempt.
- `auto_on_pass`, `manual_on_pass`, and `draft_only` release decisions.
- Publication of a verified artifact to a controller-owned local destination.
- Single-node service packaging suitable for unattended use on a Mac mini or
  Linux server.

Do not build a general gate expression language. Add a typed gate implementation
when a real workgroup needs one.

#### Deferred

Credential-bearing tools, write-capable business integrations, multiple
executors, AgentCore, browser automation, generic MCP routing, and a team UI.

#### Exit criteria

Demonstrate these real scenarios with stored evidence:

1. pass and automatic completion;
2. deterministic failure and automatic retry;
3. retry followed by completion;
4. exhausted retry followed by `NEEDS_HUMAN`;
5. controller restart during an active job followed by recovery;
6. executor inability to read controls, golden data, credentials, or other job
   artifacts; and
7. replacing the lifecycle placeholder with the real LLM executor without a
   kernel or gate change.

### Phase 3 — Build the Evaluation and Improvement Core

#### Objective

Complete the human-controlled learning loop without allowing evaluation or AI
optimization to rewrite active controls.

#### Deliverables

- Physically and logically separate storage for:
  - golden cases;
  - benchmark suites;
  - operational feedback;
  - production audit samples;
  - evaluation results; and
  - change proposals.
- An offline golden runner that submits dataset cases through the same package,
  executor, artifact, and evaluation contracts as normal work.
- Score records linked to golden version, policy version, executor version,
  evaluator version, and artifact digest.
- Deterministic evaluators first.
- A human queue for subjective rubric items.
- Production sampling and feedback ingestion as evidence, not truth.
- Proposal generation from recurring failures, score regressions, audit results,
  and human feedback.
- A human-only promotion flow:

```text
evidence
  -> change proposal
  -> human review
  -> candidate control version
  -> golden regression
  -> human promotion
  -> active control version
```

- Before-and-after evaluation reports that never overwrite historical scores.

#### Deferred

LLM-as-judge as an authoritative scorer, automated prompt optimization,
A/B traffic splitting, autonomous golden creation, and automatic control
promotion.

#### Exit criteria

- One accepted operational failure is deliberately classified and promoted to a
  new golden case by a human.
- One proposed control change is evaluated against both old and new golden
  versions.
- A human promotes or rejects the candidate with a recorded decision.
- No evaluator or executor has a write path to active controls.

**The core product is complete at the end of Phase 3.** It can run continuously,
delegate work in isolation, enforce human-authored completion policy, retain
evidence, evaluate quality, and improve through human promotion.

### Phase 4 — Add an AgentCore Provider

#### Objective

Use managed infrastructure where it reduces operational burden without moving
authority out of Chun-su.

#### Deliverables

- `AgentCoreRuntimeAdapter` implementing the same executor port.
- Mapping of capability grants to AgentCore Gateway and Identity where useful.
- Mapping of tool-call restrictions to AgentCore Policy where useful.
- OpenTelemetry export to AgentCore Observability without replacing the Chun-su
  event ledger.
- Optional export of evaluation datasets and results to AgentCore Evaluations.
- Provider conformance evidence showing that the same WorkPackage produces the
  same ArtifactBundle contract through local and AgentCore adapters.

#### Mandatory restrictions

- Use pinned production endpoints or versions.
- Disable or reject invocation-time overrides of prompt, tools, skills, model,
  and limits unless the pinned workgroup policy explicitly allows them.
- Do not permit a direct path around the controlled tool gateway.
- Do not place release credentials in the AgentCore execution role.
- Keep artifact gates and release decisions in Chun-su.
- Treat AgentCore Evaluation results as evidence, not release authority.
- Treat AgentCore Optimization output as a `ChangeProposal`, never an active
  update.

AgentCore remains modular infrastructure. Runtime, Gateway, Policy, Identity,
Observability, Evaluations, or Optimization can be adopted independently when
they provide concrete value.

Relevant official references:

- [Amazon Bedrock AgentCore overview](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/what-is-bedrock-agentcore.html)
- [Harness and Runtime comparison](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/harness-vs-runtime.html)
- [Runtime sessions](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/runtime-sessions.html)
- [Policy concepts](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/policy-core-concepts.html)
- [AgentCore Evaluations](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/evaluations.html)
- [AgentCore Optimization](https://docs.aws.amazon.com/bedrock-agentcore/latest/devguide/optimization.html)

#### Exit criteria

- Switching from local execution to AgentCore requires configuration and an
  adapter selection, not a lifecycle or policy rewrite.
- Provider loss, timeout, and malformed output follow existing retry and
  escalation semantics.
- AgentCore cannot promote controls or independently release an artifact.

### Phase 5 — Add Capabilities Only When Demand Exists

These modules are independent additions. Their order should be decided by real
workgroup needs rather than platform completeness.

#### Tool and identity expansion

- credential-brokered tools;
- MCP or API gateways;
- per-tool identity and audit;
- temporal or aggregate tool policies; and
- human approval immediately before irreversible calls.

#### External release adapters

- email or document publication;
- repository and deployment operations;
- transactional prepare/commit patterns;
- idempotency keys and compensation; and
- release-specific approvals.

#### Noel knowledge integration

Add Noel behind a versioned, read-only `KnowledgeProvider`. Record retrieved
document identifiers and versions in the package or attempt evidence. Do not use
runtime memory as an alternative source of active standards.

#### Team operation

- authentication and role-based authorization;
- remote submission and review UI;
- multiple controller workers;
- a transactional database and object store;
- backups, retention, and disaster recovery; and
- organization-level audit export.

#### Advanced evaluation and optimization

- advisory LLM judges;
- blind pairwise comparison;
- evaluator calibration;
- cost, latency, and robustness benchmarks;
- A/B experiments; and
- AI-generated prompt, rule, schema, skill, and tool-description proposals.

None of these capabilities may bypass the same human-only promotion boundary.

## 11. Recommended Initial Repository Shape

```text
control/
  workgroups/
    <name>/
      workgroup.yaml
      rules/
      skills/
      schemas/
      gates.yaml
      retry.yaml
      release.yaml
      evaluation.yaml
  evaluation/
    golden/
    benchmarks/
    rubrics/

src/chunsu/
  controller/
  contracts/
  state/
  packaging/
  executors/
  gates/
  release/
  evaluation/
  stores/
  api/

var/                       # runtime data; never executor-mounted
  state/
  packages/
  artifacts/
  evidence/
  proposals/
```

The exact package layout may follow the chosen implementation language, but the
control, runtime state, immutable artifacts, evidence, and proposals must remain
separate concepts.

## 12. Core Completion Checklist

Stop adding platform features until all of the following are true:

- [ ] Active controls are immutable to the executor and evaluator.
- [ ] Safe work completes automatically when deterministic criteria pass.
- [ ] Failed work retries or escalates only according to pinned policy.
- [ ] Every attempt runs in a fresh isolated environment.
- [ ] Controller restart does not lose, duplicate, or silently complete work.
- [ ] Every result links to an exact policy version, package digest, artifact
      digest, and executor identity.
- [ ] Golden sets, benchmarks, audits, feedback, and gate verdicts are stored as
      different evidence types.
- [ ] Evaluation results cannot change a completed job's historical verdict.
- [ ] AI-generated improvements remain proposals until human promotion.
- [ ] A second executor provider can be added without changing kernel semantics.
- [ ] The framework can run unattended on one persistent host.

The design should remain intentionally incomplete beyond this checklist. A
small stable kernel with explicit ports is more valuable than an early imitation
of every feature offered by a managed agent platform.
