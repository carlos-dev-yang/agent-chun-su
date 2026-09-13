# CHAT-02 — Telegram reaction acknowledgment

The user selected 👀 on the original incoming message instead of the fixed
`접수했습니다.` Telegram message. The host calls `setMessageReaction` before
the AI turn, with no large animation. This acknowledgment does not invoke an LLM.
Local terminal chat retains its existing text acknowledgment.

Reaction delivery shares the existing outbound serialization, minimum interval
and provider `retry_after` handling. A false, malformed or failed API result
does not authorize AI execution. The durable acknowledgment status is preserved;
reaction acknowledgments leave the historical outgoing `ack_message_ids` empty.
No automatic retry, text fallback, receipt migration or provider configuration
change is introduced. Fixed commands and actual replies still use messages.

## Validation

- `go test ./internal/telegram ./internal/telegramchat ./internal/errorreport ./internal/cli`
  passed package compilation; these packages contain no test files.
- `go vet` for those same four packages passed.
- `go build -o bin/chunsu ./cmd/chunsu` succeeded and updated the ignored local binary.
- `bin/chunsu telegram --help` succeeded without accessing the bot credentials.
- Independent Terra review found no actionable introduced bugs in the reaction,
  receipt gate, shared rate limiter, CLI preservation and matching documentation.
- `git diff --check` passed.

No new test code is authorized or added. Live Telegram delivery and running
receiver restart are not part of the recorded checks. Current host credential
access was not rechecked in this work; historical CHAT-06 handoff evidence is
separate from validation of the new reaction behavior.

Provider contract: [Telegram setMessageReaction](https://core.telegram.org/bots/api#setmessagereaction).

## Authorized receiver restart

On 2026-09-13 the user explicitly requested the restart. Before restarting,
`telegram status` showed the paired supervisor and receiver alive, connected
polling and no active AI update. The service used this checkout's `bin/chunsu`.

`telegram restart` returned success, but subsequent status showed both previous
processes stopped and launchd no longer had the service loaded. A subsequent
`telegram start` restored the service. Fresh process identities confirmed a new
supervisor and receiver running the updated checkout binary. At 09:57 UTC,
`telegram status` showed enabled intent, both processes alive, `poll: connected`,
`ai_blocked: false` and no unpersisted errors. No credential workaround was needed.

This supersedes the earlier statement that receiver restart was not checked.
No new user DM was sent during this check, so actual 👀 reaction delivery remains
unverified. The restart command's transient stop/start behavior was recovered
operationally; its implementation was not changed in this task.
