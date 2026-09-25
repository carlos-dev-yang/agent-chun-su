# WEB-01 local validation — 2026-09-25

Scope: public read-only host search/open implementation on macOS, using a
disposable private Chun-su data home. No EC2 connection, provider key, browser
package or paired Telegram account was used.

## Checks actually run

- Built `cmd/chunsu` locally and initialized a disposable data home.
- `/web status` reported disabled and no Brave key; `/web enable` changed the
  saved setting while leaving browser automation disabled. `/web disable`
  then made a subsequent public URL request unavailable. The main
  `config.json` retained version 1 without a new web field; web settings went
  to `state/web.json` for rollback compatibility.
- `/web open https://example.com` returned the observed final URL, title,
  text, retrieval timestamp, hash and `truncated=false`.
- Requests for `https://127.0.0.1`, `https://10.0.0.1`, the IPv4 EC2 metadata
  address, and `https://[fd00:ec2::254]/` were unavailable. A public
  `httpbin.org` 302 redirect to `https://127.0.0.1/` was also rejected as
  non-public. `http://example.com` was rejected by the HTTPS-only contract.
- Interrupted `/web open https://httpbin.org/delay/10` with Ctrl-C; the host
  reported cancellation instead of continuing the request.
- `/web search OpenAI latest release` returned a missing-key status without
  contacting a provider. `/web limit 0` returned the allowed 1–1000 range,
  and the saved value remained 20.
- `go test ./cmd/... ./internal/... ./setup-skills ./workgroups` passed;
  `go vet` on the same packages passed; `git diff --check` passed.

## Not run and remaining evidence

- A real Brave API request was not run because this machine has no configured
  key. Provider response/cost and daily-ledger behavior need acceptance after
  secure key provisioning.
- No reception-model `web_search`→`web_open`→cited reply was run. The local
  Codex CLI is 0.155.0-alpha.16.4, whereas reception is pinned to earlier
  validated versions; bypassing that boundary was not justified for this test.
- No EC2/AL2023, paired Telegram delivery, browser isolation, or JavaScript
  page acceptance was run. These are separate rollout checks.
- `go test ./...` failed only because ignored `var/.../baseline` snapshots are
  picked up as incomplete Go packages. Product package tests listed above
  passed; the snapshots were not changed or removed.
