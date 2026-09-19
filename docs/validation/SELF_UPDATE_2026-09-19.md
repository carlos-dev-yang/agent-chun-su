# UPDATE-01 validation checkpoint

Status: focused implementation and isolated Linux checks passed; initial EC2 rollout pending.

## Production baseline

Read-only checks on the existing Amazon Linux 2023 EC2 installation on
2026-09-19 confirmed the source checkout was clean at `1ac6d24`, the managed
controller was ready, worker dispatch was requested and running with no active
job, Telegram was paired/enabled with live receiver/supervisor and connected
polling, and the independent monitor was fresh with ready front/backend health.
The update feature had not been installed at this checkpoint.

Private model settings, bot binding, credential-store key and speaking-style
instructions remain outside the repository. Their values are not validation
artifacts and must not be copied into this document.

## Performed checks

- Existing focused Go checks passed for updateguard, selfupdate, CLI, reception,
  Telegram chat, store, backend, service, runner and operations monitor. Runner
  contains executable tests; the other listed packages currently report no test
  files, so their result establishes compilation. Focused `go vet`, shell syntax,
  package-script Python parsing and whitespace checks passed. No test source was added.
- Native Ubuntu arm64 with a systemd user manager used a private disposable Git
  origin/checkout and data homes. Non-master and dirty checkouts were refused.
  A fresh configured home completed an unchanged-master request without activation.
- Independent transient user services fetched pinned master commits, built
  separately, replaced the registered executable and restarted the existing
  controller and monitor. The updater exited successfully; it did not replace
  its own detached runner in place of the installed command.
- An explicitly stopped worker remained stopped. A separate activation with a
  configured Astra task route preserved requested/running worker dispatch and
  the exact configuration digest. No provider inference was needed for this check.
- Changing an embedded migration's content caused refusal before activation.
  Installed binary/configuration digests and controller/monitor PIDs remained
  unchanged. A deliberately failing build command likewise preserved the binary,
  settings and existing services.
- A temporary systemd condition denied candidate controller startup after the
  executable swap, then allowed the rollback startup. The updater recorded
  `failed/rolled_back`; the installed binary digest matched the original and the
  controller and monitor were running again with stopped-worker intent preserved.
  The temporary condition was removed and the activation marker was cleared.
- Independent Terra review checked the changed activation/rollback paths and
  verified the final Telegram lifecycle-lock correction did not introduce a
  reentrant lock or service-start deadlock.

Development checks found and corrected an executable-size limit and missing
lock-directory initialization before production deployment. A fresh-home run
was performed after the lock fix; the earlier fixture workaround is not the
evidence for that fix.

## Not performed / limits

Real Telegram-triggered update acceptance and completion delivery require a
real owner message. A local CLI update exercise alone does not establish that
external delivery path. Host reboot/power-loss recovery is a separate check.
The native fixture had no paired Telegram account, so its activation did not
exercise live Telegram quiesce/reconnect. Active-job refusal was not separately
fault-injected; existing controller admission checks/tests were retained.
Monitor activation verification checks the registered service is active; its
normal fresh observation was inspected after native success, not made an
additional activation gate. No database schema migration was applied.
