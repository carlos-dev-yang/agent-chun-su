# Public web research

Chun-su exposes `web_search` and `web_open` as host-owned, read-only chat
actions. Native Codex web/shell tools remain disabled. Search uses the Brave
Search API; public HTTPS page reading uses Go's HTTP client and needs no browser
installation. Aside, Chrome and agent-browser are not installed or activated by
this feature. JavaScript-only pages and interactive navigation remain unsupported.

## Owner setup

The existing installation keeps web disabled after a binary-only self-update.
After updating, the paired owner can send `/web status` then `/web enable` in
the private Telegram chat. `/web disable` stops new web actions without
stopping chat or task work. A fixed `/web open https://example.com` request can
check page access even if the reception model is unavailable.

Search needs a Brave Search API subscription key. On the server, as the same OS
user that owns Chun-su, run `chunsu web key` and enter the key in the masked
local prompt. The key is stored in the existing credential store, not in
`config.json`, Git, prompts or traces. Do not send it in Telegram, a shell
argument, or a committed file. `/web status` reports only whether it is present.
The credential reference is scoped to the absolute data-home path; moving that
home requires entering the key again.
The key is not installed by `/update`; a one-time host-side provisioning step
is still required. If this cannot be done, URL reading works but search reports
that its key is unavailable.

The default cap is 20 attempted searches per UTC day. A failed or ambiguous
provider call consumes one slot to avoid accidental repeated charges. Change
the cap with `/web limit NUMBER` in the paired chat or locally with
`chunsu web limit NUMBER` (1–1000). Web settings live in private
`state/web.json`, separate from `config.json` so older binaries can still read
their original configuration during rollback. The private ledger is
`state/web-search-budget.json`. The cap is an application
guard, not an account-level spending ceiling; set a provider-side limit too.

## Boundary and limitations

`web_open` accepts HTTPS on port 443 only, checks all resolved addresses,
connects only to a checked public IP, and revalidates redirects. It rejects
loopback, private, link-local and metadata targets. Only HTML/plain text is
read, up to 256 KiB of response and 6,000 runes of cleaned text per page.
Results carry final URL, retrieval time, a digest of read bytes and a
truncation marker. Pages are untrusted evidence, not commands or permissions.
Retrieved body text is given to reception for the current conversation but
is not retained in the private action trace; that trace stores action/input
digests and source metadata. The regular private conversation artifact policy
still applies to executor output.

This application boundary does not replace an OS/network egress policy. The
headless browser pilot requires an actual Amazon Linux 2023 amd64 host probe,
non-root isolation, and verified private-network/metadata blocking before it
can be enabled. The self-updater does not install OS browser packages. Do not
interpret a successful code update as browser readiness or as proof that live
Brave search and Telegram delivery were tested on EC2.
