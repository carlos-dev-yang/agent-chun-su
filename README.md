# Chun-su

A terminal-first personal AI workflow controller built with Go and embedded SQLite.
Human-owned workgroup assets stay outside the executor. Mail reporting comes first;
Jira is outside the current implementation scope.

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

After choosing the existing Codex account, configure `executor.kind` as `codex` and `executor.path` as the discovered absolute executable path. The adapter currently requires Codex 0.153.4. Account sign-in belongs to Codex; Chun-su does not copy its credentials. `run JOB_ID` executes one attempt, while `worker` owns the queue in the foreground. `cancel`, `retry` and `resolve JOB_ID ANSWER` preserve the earlier attempt. Queueing and cancellation reach the current owner through a private local socket.

Real Gmail disclosure is disabled by default. After a synthetic walkthrough verifies the actual executor tool boundary and the human approves the account/configuration, explicitly set `executor.live_mail_approved` to `true`. Changing executor kind/path/model or restoring a backup resets this setting. It records human authorization, not automated proof of isolation. The local feature inspection still reports `unified_exec` enabled despite its disable override; `shell_tool` is disabled. Actual tool exposure must be resolved before live use. Pin `executor.model` for comparable evaluation runs; implicit model defaults are treated as a comparison limitation.

`mail validate-report INPUT.json REPORT.json` performs offline contract checks only. It does not verify tool execution or semantic quality and cannot complete a job. Source examples are synthetic; personal expectations and a live Gmail pilot remain to be reviewed.

## Feedback and controlled changes

Use `feedback import` for versioned cases/rubrics, result evaluations, feedback and independent optimization findings. `feedback compare` keeps failures and unknown judgments visible. `workgroup propose` stores an inactive guide/schema version, `workgroup diff` shows the concrete before/after assets, and `experiment` queues the same saved input with that candidate. `workgroup decide` records adoption, rejection or rollback. AI source tools expose none of these control operations.

The initial rubric in `examples/evaluation/` is a draft. Actual personal expectations and a useful first improvement loop still require human review. Run `setup` after an upgrade to apply supported additive schema changes; unknown future schemas are never reset.

## Gmail pilot

See [Gmail pilot setup](docs/setup/GMAIL_PILOT.md) before connecting. `gmail connect` requires a local Desktop app client JSON file, the exact account and an explicit query. It uses the system browser once and stores credentials in macOS Keychain. `gmail check` verifies account identity, `gmail collect` preserves a bounded snapshot, `gmail queue` admits it once, and `gmail review` collects and runs a report. `--resume` continues interrupted retrieval; `--continue` follows a preserved page token at its original as-of boundary. `gmail reauth` repairs the same connection identity without expanding its policy.

`gmail disconnect` disables local access and removes credential references; `--revoke` additionally requests OAuth revocation from Google. Local reports remain available. `gmail normalize` inspects a saved API message without an account. The current host's Keychain rejected the non-production storage check with authentication error -25293, so successful credential storage and the live pilot remain pending.

## Reports and local recovery

`report JOB_ID` reads the verified Markdown result; `--path` prints its path. `publish JOB_ID` recovers a failed presentation stage from preserved structured output and source evidence, without another AI run. Neither command infers that a person acknowledged the report.

Use `backup NEW_DIRECTORY`, `verify-backup DIRECTORY`, and `restore BACKUP_DIRECTORY NEW_DATA_DIRECTORY`. Backups include a consistent SQLite image and declared evidence files, with hashes. Restore requires a separate new directory and leaves connections disabled and pending jobs awaiting review. Credential values are never backed up; account reconnection is explicit.

## Optional repeated operation

See the [local operations runbook](docs/setup/LOCAL_OPERATIONS.md) for pause/resume, blocked work, scheduling, the optional macOS user service, and reviewed run-content retention. Schedules are disabled at creation; `service render` shows the configuration without activating it. Restored data has admission paused and every schedule disabled. Run-content deletion requires a concrete `retention plan` followed by explicit application and preserves a tombstone; acquisition/evaluation/backup copies remain outside its scope.

Core and Phase 2–5 command paths are implemented, with bounded synthetic checks recorded under [validation](docs/validation/). Actual executor behavior, Gmail OAuth and reporting quality, service activation and MS1–MS3 milestone acceptance remain pending. No Jira connector or later-phase distribution/development workflow is included.

The [implementation handoff](docs/implementation/STATUS_2026-09-06.md) records the prepared scope, verified evidence and remaining inputs.
