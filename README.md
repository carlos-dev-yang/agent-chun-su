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

`mail validate-report INPUT.json REPORT.json` performs offline contract checks only. It does not verify tool execution or semantic quality and cannot complete a job. Source examples are synthetic; personal expectations and a live Gmail pilot remain to be reviewed.

## Feedback and controlled changes

Use `feedback import` for versioned cases/rubrics, result evaluations, feedback and independent optimization findings. `feedback compare` keeps failures and unknown judgments visible. `workgroup propose` stores an inactive guide/schema version, `workgroup diff` shows the concrete before/after assets, and `experiment` queues the same saved input with that candidate. `workgroup decide` records adoption, rejection or rollback. AI source tools expose none of these control operations.

The initial rubric in `examples/evaluation/` is a draft. Actual personal expectations and a useful first improvement loop still require human review. Run `setup` after an upgrade to apply supported additive schema changes; unknown future schemas are never reset.
