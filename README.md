# Chun-su

A terminal-first personal AI workflow controller built with Go and embedded SQLite.
Human-owned workgroup assets stay outside the executor. Mail reporting comes first.
Jira supports the approved bounded [live reporting workflow](docs/setup/JIRA_REPORTING.md)
with explicit Skills and independent evaluation; human usefulness and recurring
activation remain pending. See [service connection preparation](docs/setup/INTEGRATIONS.md)
for the six-service setup manuals and offline guided planner. Of those six,
only Gmail currently has a native Chun-su connector.

Start natural AI chat with `bin/chunsu chat` or `bin/chunsu chat '회의 준비 도와줘'`.
It uses the configured Codex executor and maintains conversation context. The model
can draft text and propose actions; the host only dispatches its declared capabilities:
status, saved-job/report lookup, setup manuals, and local Gmail setup/verification.
Unsupported actions receive extension guidance. The report executor is unchanged.
Use `chat --guided` for setup questions without an AI account. A separate setup
host starts when needed; an existing worker is reused. Of the six services, the
other five still install local guidance only. See the
[AI chat checkpoint](docs/validation/AI_CHAT_2026-09-11.md) and
[chat contract](docs/contracts/chat-v1.md).

## Development

The module selects Go 1.26.8 automatically. Dependencies are pinned in `go.mod`
and `go.sum`. No database server or SQLite CLI is required by the application.

```sh
go build -o bin/chunsu ./cmd/chunsu
bin/chunsu --help
bin/chunsu setup
bin/chunsu doctor
```

The default data directory comes from the OS user configuration directory. Use
`--home <private-directory>` or `CHUNSU_HOME` for a separate data root. Setup
preserves existing settings. Runtime data and credentials do not belong in Git.

Currently implemented commands are listed by `--help`. The remaining workflow
commands and milestones in the [implementation plan](IMPLEMENTATION_PLAN.md)
are not claims of completed functionality. No remote account is required for
initial setup. Executor and Gmail connection steps are introduced separately.

Use `config show` and `config set KEY VALUE` for non-secret settings. Use `--json`
for structured inspection. A saved JSON input can be admitted using `queue`;
the original file is copied into private storage before the job is published.

Development follows [AGENTS.md](AGENTS.md): bounded local commits, relevant
validation, and no new test code unless explicitly requested.

## Saved mail

Initialize a private data directory, inspect an example, and queue it:

```sh
bin/chunsu setup
bin/chunsu mail inspect examples/mail/synthetic-day.json
bin/chunsu queue examples/mail/synthetic-day.json
bin/chunsu jobs
```

Configure `executor.kind` as `codex`, `executor.path` as the discovered absolute executable path, and `executor.model` as `gpt-5.5`. The verified adapter combination is Codex 0.153.4 with GPT-5.5; another version/model needs boundary revalidation. Account sign-in belongs to Codex; Chun-su does not copy its credentials. `run JOB_ID` executes one attempt, while `worker` owns the queue in the foreground. `cancel`, `retry` and `resolve JOB_ID ANSWER` preserve the earlier attempt. Queueing and cancellation reach the current owner through a private local socket.

Real Gmail disclosure is disabled by default. After a synthetic walkthrough verifies the actual executor tool boundary and the human approves the account/configuration, explicitly set `executor.live_mail_approved` to `true`. Changing executor kind/path/model or restoring a backup resets this setting. It records human authorization, not automated proof of isolation. The [actual walkthrough](docs/validation/EXECUTOR_BOUNDARY_2026-09-06.md) verified the pinned combination on the pilot macOS host. The adapter rejects macOS temporary data roots because OS profile probes found a temporary-directory write exception; use the normal private application directory.

`mail validate-report INPUT.json REPORT.json` performs offline contract checks only. It does not verify tool execution or semantic quality and cannot complete a job. Source examples are synthetic; personal expectations and a live Gmail pilot remain to be reviewed.

## Feedback and controlled changes

Use `feedback import` for versioned cases/rubrics, result evaluations, feedback and independent optimization findings. `feedback compare` keeps failures and unknown judgments visible. `workgroup propose` stores an inactive guide/schema version, `workgroup diff` shows the concrete before/after assets, and `experiment` queues the same saved input with that candidate. `workgroup decide` records adoption, rejection or rollback. AI source tools expose none of these control operations.

The initial rubric in `examples/evaluation/` is a draft. Actual personal expectations and a useful first improvement loop still require human review. Run `setup` after an upgrade to apply supported additive schema changes; unknown future schemas are never reset.

## Gmail pilot

See [Gmail pilot setup](docs/setup/GMAIL_PILOT.md) before connecting. `gmail connect` requires a local Desktop app client JSON file, the exact account and an explicit query. It uses the system browser once and stores credentials in macOS Keychain. `gmail check` verifies account identity, `gmail collect` preserves a bounded snapshot, `gmail queue` admits it once, and `gmail review` collects and runs a report. `--resume` continues interrupted retrieval; `--continue` follows a preserved page token at its original as-of boundary. `gmail reauth` repairs the same connection identity without expanding its policy.

`gmail disconnect` disables local access and removes credential references; `--revoke` additionally requests OAuth revocation from Google. Local reports remain available. `gmail normalize` inspects a saved API message without an account. The selected pilot account has completed read-only OAuth, credential storage, refresh and one 20-message collection. Two real candidate reports used that same partial input; no additional page was collected. Human usefulness remains unreviewed.

The [pilot reporting rules](docs/contracts/mail-pilot-reporting.md) separate work categories from notification channels, exclude promotional content with auditable reasons, and retain uncertain operational requests. For a trial, use `workgroup propose` and `experiment BASELINE_JOB --candidate DIGEST`, then run the experiment job. Merely supplying `run --candidate` is not the experiment lifecycle. Experiments preserve source coverage and active rules; adoption remains a human decision.

## Jira collection preparation

`jira inspect examples/jira/saved-cloud.json` validates a synthetic source
envelope. After setup, `jira collect` with that file preserves and normalizes a
bounded batch. `jira resume ACQUISITION_ID` follows the pinned saved continuation;
`jira show ACQUISITION_ID --snapshot` shows normalized facts and gaps. Use
`jira source ACQUISITION_ID 0` to inspect exact source bytes and
`jira renormalize ACQUISITION_ID --mapping MAPPING.json` to create a new derived
record while preserving the earlier one.

No account or executor is needed for saved-input ingestion. For configured live
acquisition and report execution, use the [Jira reporting guide](docs/setup/JIRA_REPORTING.md).
See the [terminal guide](docs/setup/JIRA_INGESTION.md),
[evidence contract](docs/contracts/jira-evidence.md), and
[saved-input checks](docs/validation/PHASE_06_JIRA_INGESTION.md). The later
[live/Skill checkpoint](docs/validation/JIRA_SKILL_INTEGRATION_2026-09-08.md)
records the approved integration and its remaining limits.

## Reports and local recovery

`report JOB_ID` reads the verified Markdown result; `--path` prints its path. `report JOB_ID --sources` reads the corresponding local source document, and can also use `--path`. New Markdown artifacts have `.md` filenames; old artifact paths remain valid. The summary links to deterministic source labels, while the source document retains normalized text, origin metadata and exclusion reasons. It is not an original EML or attachment archive. `publish JOB_ID` recovers a failed presentation stage from preserved structured output and source evidence, without another AI run. Neither command infers that a person acknowledged the report.

Use `backup NEW_DIRECTORY`, `verify-backup DIRECTORY`, and `restore BACKUP_DIRECTORY NEW_DATA_DIRECTORY`. Backups include a consistent SQLite image and declared evidence files, with hashes. Restore requires a separate new directory and leaves connections disabled and pending jobs awaiting review. Credential values are never backed up; account reconnection is explicit.

## Optional repeated operation

See the [local operations runbook](docs/setup/LOCAL_OPERATIONS.md) for pause/resume, blocked work, scheduling, the optional macOS user service, and reviewed run-content retention. Schedules are disabled at creation; `service render` shows the configuration without activating it. Restored data has admission paused and every schedule disabled. Run-content deletion requires a concrete `retention plan` followed by explicit application and preserves a tombstone; acquisition/evaluation/backup copies remain outside its scope.

Core and Phase 2–5 command paths and the approved Phase 6 saved ingestion subset are implemented. [Validation records](docs/validation/) distinguish synthetic checks, actual restricted executor runs and the bounded live Gmail candidate pilot. Broader lifecycle/reliability checks, personal reporting quality, service activation and MS1–MS4 milestone acceptance remain pending. Live Jira access, Jira AI reporting and later-phase distribution/development workflows are not included.

The [latest readiness check](docs/implementation/STATUS_2026-09-07.md) records the project structure, rerun saved Jira checks and remaining connection inputs. The [Jira connection preparation](docs/setup/JIRA_CONNECTION_PREPARATION.md) separates required deployment/scope decisions from later metadata discovery. The [September 6 handoff](docs/implementation/STATUS_2026-09-06.md) preserves the earlier execution evidence.
