# Agent drivers and execution environments v1

`executor.Driver` selects the AI provider adapter. `runtimeenv.Environment`
selects the execution boundary. Reception and review request structured output
with explicit roles; task execution uses its separately prepared package. The
first compiled driver is Codex and the first environment is `native-restricted`.
Unknown drivers/environments are rejected; they are not shell commands or
configuration-defined plugins.

The temporary Codex stabilization policy requires CLI 0.153.4 / gpt-5.5 for
task and review routes. The reception route also accepts CLI
0.154.0-alpha.6.2 / gpt-5.5 on macOS/arm64, following the bounded
[Telegram compatibility revalidation](../validation/TELEGRAM_CODEX_COMPATIBILITY_2026-09-13.md).
Other versions, models and the new combination on other platforms still require
their own revalidation. Compatibility inspection uses the selected role, so a
matching reception route does not make task or review execution eligible.
The core configuration, reception and review logic no longer require that
specific provider. Supporting another driver or compatibility combination
requires adapter/enforcement evidence before enabling it. A stored new model
name does not silently opt out of the existing driver's validation.

The environment resolves a dedicated package directory inside the selected
data root and declares its read roots, no tool write roots and disabled tool
networking. The Codex adapter translates the boundary into its native permission
profile. Required system runtime reads and the known macOS temporary-directory
exceptions remain properties of that native mechanism; the data root cannot
be placed in those macOS exceptions. This is not a VM/container claim. Provider
authentication/inference traffic remains distinct from model-tool network
permissions. The trusted CLI and source-gateway processes still use their own
host functions; the model cannot read the controller through its tools.

Actual executions preserve role, driver, canonical boundary, CLI version,
argument digest and existing process identity. Workgroup manifests still pin
the selected configuration and Skill. `doctor` reports each route as unavailable,
needing revalidation or having matching prerequisites. Matching prerequisites
does not prove a new node's isolation or company-account access.

## Role configuration

Existing configuration continues to use `executor` for all roles. Optional
`routes.reception` and `routes.review` select complete independent routes. A
route change clears live-disclosure approval on that explicit route.

```sh
chunsu config route task --driver codex --path "$CODEX_PATH" --model gpt-5.5 --environment native-restricted
chunsu config route reception --model gpt-5.5
chunsu config route review --model gpt-5.5
chunsu config route review
chunsu config route review --inherit
chunsu doctor
```

Task-route changes affect inheriting roles but do not rewrite explicit routes.
The existing live-source approval flow currently applies to the task route;
use inherited review routing for that reviewed selection. Explicit live-review
routes require their own approved configuration and source policy. No CLI flag
copies old approval to a new model or silently grants one. Backup restoration
revokes disclosure on every route, including explicitly configured roles.

## Recovery

The single writer reconciles saved task and review process identities before
recovery. A known surviving process group is terminated and confirmed; an
interrupted start with no recorded identity requires inspection. Pending review
requests become immutable interrupted executions, without importing a judgment
or replaying a task. Partially generated evidence stays available. A completed
review execution with no imported evaluation can be inspected explicitly; it
is not accepted automatically after a restart.

Foreground review retains the current single-writer rule. If a worker or setup
host owns the root, a second writer is refused; stop that owner or use the
appropriate management path. Multiple remote writers must never share SQLite.
