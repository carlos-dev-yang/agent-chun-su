# CHAT-TRACE-01 — Message routing and saved host results

The shared reception process suppresses pre-action model prose and technical host
result lines. Final action-free replies/questions remain user-facing. Local
authentication/report callbacks, fixed commands, safe failures and Telegram 👀
acknowledgments retain their existing user interaction paths.

Each generated step now records `message.json` with kind, audience, text digest,
request/session/step correlation and delivery state. `action.json` preserves a
bounded allowlisted host result, action intent/outcome, timestamps, correlation,
original result digest and saved-result digest. Telegram calls also supply their
update ID. Callback-only authentication/report results save status only; message
prose, auth inputs, report bodies and raw errors are excluded. Known public host
guidance and diagnostics may be retained as part of the projected result.

## Checks performed

- Focused `go test` for reception, conversation, telegramchat and CLI passed
  compilation; these packages contain no test files.
- `go vet` passed for the same packages; `go build -o bin/chunsu ./cmd/chunsu`
  succeeded, including after the final internal-emitter adjustment.
- Actual local Codex conversation requested a read-only runtime status. The
  terminal showed the existing local acknowledgment and one final answer, with
  no pre-action prose or technical host-result line.
- The status action saved `observed` and structured runtime result fields. The
  progress record was internal/suppressed, the final reply user/sent, and both
  steps shared one request ID. Action/message records matched correlation and
  used private mode 0600. Message records contained no text field.
- A subsequent action-free request produced its requested short reply, with a
  user/sent message record and no action record.
- After the final emitter adjustment, another actual runtime-status conversation
  produced one final answer; its action record explicitly had internal/suppressed
  classification, the structured result and both result digest fields.
- Independent Terra review checked trace projection, message suppression,
  persistence ordering, uncertainty and callback preservation. After removing
  unused direct internal emits and marking actions suppressed, no further
  concrete introduced issue remained.

## Limits and rollout

No new test source was created. Failure-injection, oversized-result, authentication
handoff and actual Telegram message-delivery scenarios were not executed in this
check. Source review covered those unchanged or directly affected boundaries;
live message presentation remains a separate check using the owner's next DM.

The updated binary was applied to the current Mac's Telegram service under the
user's existing restart authorization. With no active AI update observed, the
receiver was stopped, confirmed stopped, then started. Fresh supervisor/receiver
identities were observed, and at 10:17 UTC both were alive with enabled intent,
`poll: connected`, `ai_blocked: false` and zero unpersisted errors. No new DM had
arrived after restart at that checkpoint. The rollout does not claim live message
presentation verification or completion of the separate operations-monitor work.
