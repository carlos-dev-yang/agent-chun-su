# ADR 0001 — Local Core Implementation Baseline

Date: 2026-09-06. Status: implementation baseline for the user's authorization to proceed through Phase 5. Jira and later phases remain outside this run.

## Decisions

- Go + embedded SQLite is confirmed by the user. The local module is `chunsu`; no public repository identity or remote is invented.
- Use Go 1.26.8 through Go's project toolchain selection, leaving the existing Homebrew installation unchanged.
- Pin Cobra v1.10.2, modernc.org/sqlite v1.58.0, and JSON Schema v6.0.3 when their implementation is introduced. SQLite uses explicit SQL and the driver's own engine.
- Start with a manual CLI, one controller owner per data directory, private runtime files, durable requests/attempts, and local Markdown reports. Add the service and scheduler after the core works.
- Use JSON settings and Markdown workgroup assets. Derive the data directory from the OS user configuration directory; allow `CHUNSU_HOME` or an explicit CLI option to select another data root.
- Database schema v1 implements the previously specified ownership/lifecycle, not a new product workflow. Later schema additions are versioned; unsupported versions fail without resetting data.
- The user selected Gmail for the first live connector. OAuth account consent, message scope, and real-data disclosure will be requested when the connection flow is ready.

## Executor status

Codex CLI 0.153.4 is installed and reports ChatGPT login. Non-interactive mode, structured output, and restricted filesystem profiles are available. The user has been asked whether to use that executor. No model invocation or credential copy has been performed by this decision step.

If selected, isolate the work package and disable unrelated apps, plugins, browser tools, shell capabilities, and web search. Expose only host-scoped source tools. The CLI authentication belongs to the trusted executor runtime; Gmail credentials never enter its prompt, environment, or package. Validate the actual restriction before using real mail. A permission-profile configuration alone is not verification.

## Evidence and references

The local Go version was 1.26.4; the project toolchain successfully reported Go 1.26.8. Module metadata was resolved from the configured official Go module proxy. No application build or DB behavior has yet been claimed here.

- [Go release history](https://go.dev/doc/devel/release)
- [SQLite driver](https://pkg.go.dev/modernc.org/sqlite)
- [Codex non-interactive mode](https://learn.chatgpt.com/docs/non-interactive-mode)
- [Codex permission profiles](https://learn.chatgpt.com/docs/permissions)
- [Gmail Go quickstart](https://developers.google.com/workspace/gmail/api/quickstart/go)

The Gmail quickstart's simplified token-file and authorization-code example is not the production credential design. Use desktop loopback OAuth with state/PKCE and host secret storage.
