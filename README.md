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
