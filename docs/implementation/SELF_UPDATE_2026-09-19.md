# UPDATE-01 — Owner-triggered master updates

Status: implementation and focused Linux validation complete; initial EC2 feature rollout pending.

## Authorized behavior

The owner selected a chat-triggered update from the registered repository's
`master` branch. Database structure changes require further validation and are
excluded from automatic updates. The first installation of this capability is
a normal operator deployment; later updates use a fixed chat command.

- `/update check` inspects the allowed branch; `/update` requests installation;
  `/update status` reports the saved outcome. These are host commands, not AI
  shell access. Source configuration is local-only.
- Retrieve a pinned master revision and build in a separate directory while
  the current chat stays available. Preserve the source checkout and local work.
- The updater runs as the existing OS user in an independent systemd user unit,
  outside the chat service cgroup. It is a bounded one-shot, not another agent
  manager or database writer.
- A confirmed Telegram acceptance receipt precedes activation. Uncertain
  acknowledgments must not trigger installation or automatic message replay.
- Refuse activation while work/configuration operations or a chat generation
  are active. Preserve the controller, worker, chat and monitor desired states.
- Check candidate database schema and embedded migration identity before
  opening production data with the candidate. Refuse incompatible changes.
- Preserve configuration, credentials, language, tone and existing evidence.
  No Codex/model upgrade, provider sign-in, expanded disclosure, or OS security
  policy change is part of self-update.
- Retain the old executable for rollback. Replace the executable at its existing
  registered path; restart only intended services and the chat last. Health
  failure restores the prior binary when safe. Never roll back by overwriting
  the live database with a historical backup.
- Keep durable bounded update status and failure details without secret values.
  Interruptions that cannot be safely reconciled require explicit local recovery;
  this scope does not promise transparent recovery from every power failure.

## Validation required

Use existing affected Go tests/build/vet, plus isolated manual process checks.
Exercise unchanged master, changed master, busy refusal, schema mismatch,
failed build, failed activation/rollback, preserved stopped intent, and unchanged
settings. Validate that the updater survives chat service restart. Use actual
EC2 activation only with a verified candidate and preserve the existing bot
binding and owner identity. Report all unperformed checks separately.

The [validation checkpoint](../validation/SELF_UPDATE_2026-09-19.md) records
successful native activation and binary rollback, unchanged settings, preserved
running/stopped worker intent, and the deliberately unperformed real-Telegram
and power-loss checks. Configuration requires the operator to supply the full
source revision actually used to build the installed executable; it does not
infer that revision from a possibly older checkout.

## Relationship to prior work

The EC2 host is Amazon Linux 2023, so self-update must not invoke the Ubuntu-only
first-install bootstrap. Existing Linux package installation, fixed command
handling, Telegram receipts and component lifecycle controls are reused.
No new external notification destination, arbitrary branch selection, dependency
installer or database migration policy is introduced.
