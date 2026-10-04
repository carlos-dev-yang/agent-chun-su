# Public synthetic team-ops fixtures

Use these only in an explicitly synthetic `pilot/checkout` scope in a separate
private home. See the [local setup guide](../../docs/setup/TEAM_OPS_LOCAL.md) and
[actual execution/partial-result evidence](../../docs/validation/OPS_02_LOCAL_2026-10-05.md).

`saved-slack.json` is an owner-imported v1 saved envelope: a current payment failure
with no urgency keyword, an investigation that does not prove recovery, and a
historical resolved incident quoting “URGENT”. It preserves source/capture times,
workspace/channel/thread/message/event/version and exact content spans. It is not
a live Slack collector, disclosure approval or real service-health observation.

`shadow-policy.json` uses demo recipient labels only. P0/P1 immediate and P2 digest
are local plan classifications, P3 none stays local; cooldown/acknowledgment/freshness
values are explicitly zero for this fixture. There is no sender or sent status.
Select real policies/recipients only after the corresponding team/delivery contract
is implemented and approved.

Saved mail and Jira examples may also be imported without changing owner business
state. Confirm mapping rather than assuming their external issues belong to this
project. Do not relabel externally sourced or private data as synthetic.
