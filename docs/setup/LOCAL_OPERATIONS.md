# Local Operations Runbook

Date: 2026-09-06. Scope: Phase 5 implementation preparation. A useful live mail pilot and the user's operating policy are still required before unattended activation. Jira and later phases are outside this implementation.

## Working operating policy

| Concern | Implemented behavior | Personal decision still needed |
|---|---|---|
| Delivery | Verified local Markdown and terminal inspection; reading does not imply acknowledgment | Whether this is sufficient for the pilot |
| Schedule | Explicit elapsed interval; disabled at creation | Desired interval and report timezone |
| Missed intervals | Preserve the due timestamp and missed count; admit one current collection | Acceptance of coalescing after sleep/logout |
| Overlap | One controller and one active AI job per data root; one unfinished occurrence per schedule | None for the initial single-user scope |
| Pagination | One bounded page per occurrence; continue the previous page chain at its original as-of boundary before beginning another scan | Pilot query and batch size |
| Timezone | Connection policy and schedule pin an IANA report zone; eligibility uses elapsed UTC intervals | Choose a zone explicitly; default setup may use UTC |
| Retry | HTTP calls have a bounded attempt count; scheduled collection and AI attempts each use the named `limits.max_attempts` budget | Raise limits only after inspecting failures |
| Questions/authentication | Persist blocked state and evidence; a linked blocked job prevents the next occurrence | Answer/repair and explicitly resume or skip |
| Retention | Manual review and application for one finished run; no automatic expiry | What data should eventually expire, including acquisition/evaluation copies |
| Service | Optional user LaunchAgent; foreground use remains sufficient | Explicit install/start and, separately, login activation |

An interval of `24h` means 24 elapsed hours, not “09:00 every local day.” Daylight-saving changes do not create duplicated wall-clock slots. Changing the global report timezone does not silently rewrite an existing Gmail connection or schedule. If the connection timezone is edited, the schedule blocks for review. A schedule never advances source coverage by itself: only an available validated report does that. Coverage does not mean a person completed the requested action or read the report.

Collection attempts resume a durable checkpoint and retain provider `not_before` delays. Exhausting a budget needs intervention; creating a new occurrence is not an automatic way to evade it. A manually skipped occurrence is recorded as skipped, and its unfinished pagination chain is abandoned explicitly. A subsequent query may find those messages again; no exactly-once processing promise is made.

## Foreground use and intervention

Use the same `--home` on every command, or set `CHUNSU_HOME` to one private directory. Select and validate an absolute executor path before running work.

Until the binary is on your PATH, use `bin/chunsu` in place of `chunsu` below. See the [README](../../README.md) for the single-binary build/setup sequence. Real Gmail disclosure additionally requires the reviewed `executor.live_mail_approved` setting; a changed executor or restored root must be reviewed again.

```sh
chunsu status
chunsu worker
chunsu jobs
chunsu show JOB_ID
chunsu logs JOB_ID
chunsu report JOB_ID
```

`worker --once` processes at most one eligible attempt, or one scheduled collection if no job is eligible. A collected snapshot is then queued for the next worker iteration. `Ctrl-C` stops a foreground worker. The controller reconciles interrupted attempts on the next start; it never labels an interrupted run successful.

`pause` persists an admission pause and allows already active work to finish. `unpause` resumes eligibility. `cancel JOB_ID` revokes a running job's source access and cancels its executor group. These commands, plus queue/retry/resolve/status, use the private management socket when an owner is running. The same commands acquire ownership directly when no owner exists. User socket permissions supplement the executor sandbox; they are not a same-user isolation boundary on their own.

Stop the worker/service before changing config, connections, active workgroup assets, schedules, publication recovery, backups or retention. Those operations require exclusive ownership and refuse competing writes. This first version deliberately does not hot-reload operating controls into active attempts.

| Situation | Inspect | Resume the correct stage |
|---|---|---|
| Executor authentication unavailable | `show`, `logs`, executor's own login status | Repair executor login, then `retry JOB_ID` |
| Source collection authentication unavailable | `gmail acquisitions`, `gmail list` | Repair with `gmail reauth CONNECTION_ID --client CLIENT.json`; `gmail collect CONNECTION_ID --resume ACQUISITION_ID` or `schedule retry TICK_ID` |
| Provider delay/outage | Acquisition/tick `status`, `not_before`, previous failures | Wait for eligibility; automatic retries remain bounded; explicit resume after repair |
| Report asks a question or recovery requires review | Preserved output, lookup evidence and `show` | `resolve JOB_ID 'answer or recovery-review note'` |
| Attempt budget exhausted | Attempts, limits and actual need to repeat | Review a named limit or queue a separate experiment; earlier attempts remain |
| Report presentation failed | Preserved structured result, execution metadata and sources | `publish JOB_ID`; no AI rerun |
| Start intent has no recorded process identity | The attempt's `process.json` and surviving processes | Stop and inspect; never blindly kill a PID or automatically retry uncertain work |
| An occurrence should be abandoned | `schedule ticks`, linked job | Cancel active work first, then `schedule skip TICK_ID 'reason'` |

A process record in `starting` state intentionally stops automatic recovery. There is no unsafe force-recovery command. Once the operator has established that the old process group is gone, the process record can be repaired with a recorded local intervention before retry. The application does not infer this fact from missing output.

## Optional schedule and macOS service

After the manual pilot and operating policy have been accepted:

```sh
chunsu schedule add CONNECTION_ID --name 'Mail review' --every 8h
chunsu schedule list
chunsu schedule enable SCHEDULE_ID
chunsu service render
chunsu service install
chunsu service start
chunsu service status
```

The interval above is an example, not a selected personal default. `service render` only prints a reviewable plist. `install` writes a per-user definition with a data-root-derived label and absolute executable/data paths; it does not start the process. `start` registers and starts it in the current GUI login domain. `install --at-login` separately opts into future login startup. Existing conflicting definitions are preserved. A stored definition captures a small explicit environment and does not depend on shell startup or the repository working directory.

`service stop` unloads the registration; `service start` loads it again. `service remove` stops a loaded registration and removes only its verified definition and local service metadata. User data remains. To change login behavior or executable location, remove and reinstall the registration after reviewing the new rendered definition.

The first service version has no automatic crash restart (`KeepAlive=false`); inspect `service status` and explicitly start it after repair. The user agent exists only in the user's login session. It does not run while the Mac sleeps or the user is logged out, and it does not wake the Mac. There is no root daemon. A startup request succeeding is not proof that the worker remained healthy; verify status and queue state. Service stdout is not a second unbounded report log; job evidence stays in the controlled data root.

No service has been installed/started and no personal schedule enabled during implementation verification. Plist parsing and command construction have been checked; actual launchd lifecycle, sleep/wake and long-duration reliability remain unverified.

## Backup, restore and run-content retention

Stop the worker, then create a fresh destination outside the source root:

```sh
chunsu backup NEW_BACKUP_DIRECTORY
chunsu verify-backup NEW_BACKUP_DIRECTORY
chunsu restore NEW_BACKUP_DIRECTORY NEW_DATA_DIRECTORY
chunsu --home NEW_DATA_DIRECTORY status
```

The backup uses a consistent SQLite `VACUUM INTO` image that includes committed WAL contents, plus declared files and SHA-256 hashes. It excludes Keychain values, sockets, locks, transient staging files, service registration and recursive backup directories. Treat backup contents as private mail/work data. A failed creation has no completed manifest and must not be called a successful backup.

Restore requires a separate new data root, verifies references, disables connections and all schedules, resets live-mail disclosure approval, pauses admission, and changes pending jobs to require review. It creates fresh unused secret references, so reconnecting a restored copy cannot delete the original root's Keychain references. Reconnect accounts and review work before explicitly unpausing or enabling a schedule. No launchd registration is restored. Unsupported schema versions fail without resetting data.

Retention is an explicit, reviewable logical deletion:

```sh
chunsu retention plan FINISHED_JOB_ID
chunsu retention show PLAN_ID
chunsu retention apply PLAN_ID --confirm PLAN_ID
```

The plan lists exact paths, hashes and byte counts. It rejects a changed job or changed run files. Application marks content retiring before removing it, then keeps a purged tombstone and an audit event. Repeat the same plan ID to finish interrupted deletion; a completed repeat is idempotent. Purged content cannot be reported as available or reproducible. Backup verification respects those tombstones.

This operation deletes only the selected run directory and private request/event details. Acquisition checkpoints, evaluation/proposal text, source-coverage metadata, backups and external copies can still contain related data. It is not a mailbox deletion, an account-wide erasure or secure physical-media erasure. There is no automatic retention policy yet. All deletion checks performed during implementation used self-created synthetic temporary data.

## Current prerequisite checklist

- Choose the existing Codex executor account and complete the actual adapter/tool-boundary walkthrough with synthetic sources.
- Review personal mail semantics and the draft evaluation rubric; perform the first useful baseline/candidate comparison.
- Resolve macOS Keychain authentication error `-25293`, then repeat the storage check after that state change.
- Provide the Gmail test account, explicit query/batch policy and local Google Desktop OAuth client JSON path. Keep tokens and passwords out of chat.
- Complete a bounded manual Gmail pilot before accepting operating policy and activating unattended work.

See [Gmail setup](GMAIL_PILOT.md), [operations evidence](../validation/PHASE_05_OPERATIONS.md), and the [master implementation plan](../../IMPLEMENTATION_PLAN.md).
