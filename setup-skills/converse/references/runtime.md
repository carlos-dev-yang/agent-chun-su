# Runtime operation and recovery

This manual describes the local runtime after the process boundary rollout. It
is read-only guidance. Reading it does not start, stop, restart, configure, or
install anything. Reading this guide or seeing its examples alone does not
authorize an action. An explicit user request and the existing capability
controls still decide whether an action can be requested.

## Structure and ownership

- The front gateway receives authenticated chat traffic and serves common
  conversation plus fixed commands. It remains useful when the controller or
  task execution is unavailable.
- One controller owns the SQLite writer, state, queue, and task control for one
  data home. Do not run another writer for that same home.
- The worker is controller-managed task dispatch, not a second persistent
  database writer. It starts executor child processes only after task runtime
  configuration passes validation.
- The existing chat supervisor and OS user service manage process liveness.
  The monitor observes front and backend health separately. It does not restart
  work or make task decisions.

Slack adapter support is not implemented. The monitor records local incident
state only; no external notification channel is implemented.

## Fixed chat commands

Use `/status` before changing a component. `/controller status` and `/worker
status` give component-specific observations.

- `/controller start`, `/controller stop`, and `/controller restart` manage the
  known controller service. Stop or restart is rejected while active work or a
  setup flow is busy. Let active work finish or use its existing explicit
  control before retrying.
- `/worker start`, `/worker stop`, and `/worker restart` manage only new task
  dispatch. Worker stop does not arbitrarily cancel active work. Worker start
  and restart are rejected while active work or setup is busy. They can report
  missing task runtime configuration while the controller stays up.
- `/pause` and `/resume` control queue admission separately from worker state.
  Starting a worker does not resume a paused queue.
- `/errors` shows retained operational errors and recovery guidance. `/jobs`
  helps identify active or past work.

There is no force-all command and no supported command to restart every
component. Change only the component you have inspected. Do not repeat an
uncertain start, stop, or reply automatically.

## Saved worker AI settings

`/worker config` shows the saved task AI driver, model, environment adapter, and
available locally configured source routes. It works without a running
controller and does not change settings or start work. It does not reveal
executable paths, credentials, or live-data approvals.

For first-time setup, choose an existing source explicitly:

```
/controller start
/worker stop
/worker config use reception
/worker config
/worker start
```

Use `use review` instead when you explicitly want the locally configured review
route. The source's execution settings are copied as a saved snapshot, so later
changes to that source do not silently change the worker. Source credentials
and data-disclosure grants are not copied into chat or newly granted to task
work. A local executable may still use its existing local authentication.

To change the model, stop dispatch and wait for active work or setup to finish,
then use `/worker config model MODEL` with the actual model identifier you chose.
Keep new executables, account authentication, and environment registration in
the local or SSH configuration flow. Changing task execution settings clears
its existing live-data approvals; any needed approvals use their existing local
flow. An unchanged selection does not rewrite settings or clear approvals.
Existing chat and review settings retain their effective values.

Configured means settings are present, not that the selected executable/model
has passed the task runtime checks. A reception route can be valid for chat but
require separate task validation. If worker start reports prerequisites need
attention, inspect `chunsu doctor` locally for role-specific compatibility.
Changing settings does not bypass the existing version/model validation.

Saving settings requires a running controller and a stopped, idle worker. It
does not start or resume work. `/worker start` and `/worker restart` read the
saved settings each time. A controller restart also retains the saved settings
and explicit worker start/stop intent. If a save cannot be confirmed, inspect
`/worker config` and `/worker status` before deciding whether to change anything.

Natural requests can read worker settings, copy a user-selected existing source,
or change a model named by the user. A natural worker restart uses the single
`restart_worker` action. Merely reading this guide does not authorize a change.

## Initial local or SSH checks

Run commands as the same user and with the same data home as the service. For
a non-default home, replace the quoted placeholder `"<data-home>"` with the
actual path consistently. The quotes prevent the shell from treating the angle
brackets as redirection syntax; do not copy the placeholder literally.

```sh
chunsu --home "<data-home>" controller status
chunsu --home "<data-home>" controller start
chunsu --home "<data-home>" worker status
chunsu --home "<data-home>" worker start
```

If the worker reports unavailable configuration, use `/worker config` to inspect
saved settings and available source routes before trying again. Do not use chat to provide secrets,
credentials, arbitrary shell input, or an alternate host path.

When configuration needs a change, check status for active work. If a setup
flow is in progress, wait for it to finish or cancel it through its existing
controls. Controller stop rejects busy work or setup; proceed with
configuration only after stop succeeds. Then inspect the route command help,
make the user-selected configuration change, and start the controller again:

```sh
chunsu --home "<data-home>" controller status
chunsu --home "<data-home>" controller stop
chunsu --home "<data-home>" config route --help
chunsu --home "<data-home>" controller start
```

This sequence does not choose an account, executable path, model, or route for
you. Select those values explicitly through the approved local configuration
flow.

## Recovery sequence

1. Read `/status` to distinguish front-gateway health, controller availability,
   worker readiness, and queue state.
2. If the front gateway responds but the controller is unavailable, use
   `/controller start`, then check `/controller status`.
3. If the controller is available but dispatch is stopped, inspect `/worker
   status`. Start the worker only when its task runtime configuration is ready.
4. If the queue is paused, use `/resume` only when you explicitly intend to
   admit new work.
5. If a change fails or its outcome is unknown, inspect `/status`, `/jobs`, and
   `/errors`; do not force a restart or re-run a job.

If the front gateway itself is unavailable, use the same-user local or SSH CLI
with the same data home. Telegram commands inspect or restart the front
receiver; monitor commands inspect facts, saved local incidents, or only the
monitor service:

```sh
chunsu --home "<data-home>" telegram status
chunsu --home "<data-home>" telegram restart
chunsu --home "<data-home>" monitor check
chunsu --home "<data-home>" monitor status
chunsu --home "<data-home>" monitor restart
chunsu --home "<data-home>" controller status
chunsu --home "<data-home>" controller start
chunsu --home "<data-home>" worker status
```

`telegram restart` first verifies the registration and existing process
ownership. A failed precheck rejects the transition without changing intent.
It then performs one ordered operation: save stop intent, wait for
the old OS service, supervisor, receiver, and ownership locks to clear, start
the service, then confirm the new supervised receiver is connected to polling.
Telegram lifecycle changes for the same data home are serialized; status reads
remain available. A stop-confirmation failure leaves intent disabled and does
not start a replacement. A startup-confirmation failure returns an error while
intent remains enabled, so the supervisor may still recover. Inspect `telegram
status` and `errors` before deciding what to do next. Success means the new
receiver was ready at the time of the check. Restart does not restart the
controller or worker, or replay uncertain work or replies.

`/language` changes AI-generated conversational replies only. Fixed command
responses and this manual are deterministic English. It does not alter runtime
state or the meaning of commands.
