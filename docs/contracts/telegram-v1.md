# Telegram private chat v1

Historical IN-05 baseline. The authorized [persistent chat extension](chat-operations-v1.md)
supersedes the foreground-only lifecycle, fatal failure behavior and deferred recovery below.

IN-05 implements the user's 2026-09-11 request to test Telegram as the chat entry.
It is a foreground, local long-polling transport for the existing configured Codex
conversation adapter. It does not change the report executor, DB schema, report
source disclosure policy, active rules or schedule activation.

`telegram` loads or creates a bot binding, pairs if needed, then serves private
text requests. The first token comes from masked local terminal input or an explicit
private token file. The existing Keychain implementation stores it; model stdin,
arguments and artifacts never include it. Bot API URL errors and response bodies
are withheld because the provider protocol puts the token in its URL path. HTTP
redirects and inherited HTTP proxies are disabled. HTTPS is required, except an
explicit loopback HTTP fixture. Endpoint choice belongs to local setup only.

`getMe` checks bot identity; `getWebhookInfo` rejects an existing webhook without
removing it. A saved token must still identify the saved bot on every startup.
Setup uses an expiring random local `/start CODE`, requiring a non-bot sender in
a private chat whose ID matches the sender. Only that sender/chat pair is saved.
The confirmation reply and subsequent model responses go to that fixed DM. No
username-only authorization, groups, forwarded/via-bot inputs or arbitrary send
destinations are admitted. Existing saved bindings are not silently overwritten.

The receiver owns its own lock below `state/telegram/`, separate from the normal
controller writer lock. Setup/status actions reuse the existing private controller
channel; a setup-only child starts when needed and only that child is stopped on
exit. A second receiver in the same data root cannot own the Telegram lock. Another
program using the same bot may cause provider conflict; it is not automatically disabled.

Telegram's schema and prompt derive a projection of the existing capability list:
`none`, `runtime_status`, `read_guide`, `install_guide`, `list_jobs`. The projection
is checked before displaying a proposal and again at dispatch. Local Gmail auth
and report-body display are omitted. The transport does not pass token input,
connection fields or report bodies to the AI. User text and requested job summaries
are sent to the configured AI; replies and requested summaries go through Telegram.

One AI request runs at a time while polling continues for `/cancel`, `/reset` and
`/help`. A new ordinary request while busy receives an explicit busy reply and is
not queued. Cancel waits for the generation/process-group cleanup; completed host
actions remain completed. Reset clears in-memory history. Terminal chat behavior
is preserved, using the full local capability list and a local channel marker.

The receiver claims an authorized update in a private receipt before advancing its
local cursor, then acknowledges that update to the polling loop. IDs already claimed
are not replayed, including claims left uncertain by crashes. Unauthorized updates
advance the cursor without AI calls or replies. This is duplicate suppression, not
exactly-once delivery: crashes can leave an unprocessed claim that needs human review.
It deliberately does not guess that an unconfirmed provider send failed and resend.

Receipts contain update ID, state, input/latest intended reply hashes and confirmed
message IDs. They omit text and tokens. Each send persists intent first; partial or
unconfirmed sends preserve their known IDs and stop without replay. AI and host-action
metadata use the existing `chat/telegram-<session>/<turn>` records. These records do
not establish golden-set quality or release approval. The full transcript is memory
only; provider retention and terminal scrollback are outside that statement.

Replies use plain text, disable link previews, split on bounded UTF-16 units and cap
length with an explicit truncation notice. No Markdown/HTML parse mode, uploaded files,
message deletion, proactive schedule, background installation or group privileges are
added. Networking errors stop the foreground receiver for inspection. Token rotation,
re-pairing UI, receipt retention/backup restoration and hard-crash process recovery
remain follow-up work; a saved binding never proves a live session is running.

Provider behavior: [Bot API](https://core.telegram.org/bots/api),
[setup tutorial](https://core.telegram.org/bots/tutorial),
[FAQ](https://core.telegram.org/bots/faq).
