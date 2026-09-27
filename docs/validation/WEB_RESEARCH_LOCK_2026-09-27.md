# WEB-01 web lock regression — 2026-09-27

The error ID `0508257e1c8258ebfb3b944f6b7c45521206bc542316e05101806299c5f73196`
is the fixed `host_failed` category ID, not a unique occurrence identifier.
The reported `/web enable` failure was reproduced in an isolated local data
home: an active controller held `state/controller.lock`, while `/web enable`
attempted the same lock and failed after the configured five-second wait.
The search-attempt ledger had the same lock dependency.

The correction gives web settings and the ledger their own private
`state/web.lock`; settings changes and search reservations serialize on that
lock. The controller lock and controller behavior were not changed.

## Checks actually run

- Built the prior binary, started a disposable controller, confirmed it was
  active, and observed `/web enable` return the same generic error and ID
  after approximately five seconds.
- Kept that controller running, built the corrected binary, and observed
  `/web enable`, `/web limit 1`, CLI `web limit 2`, `/web status`, a public
  `/web open https://example.com`, and `/web disable` succeed. After disabling,
  a new public open returned `unavailable`. The disposable controller was
  stopped normally after the checks.
- Targeted Go tests and vet for the changed packages passed. A Linux/amd64
  `cmd/chunsu` cross-build also passed. No test source was added.

## Not yet verified

The correction has not been activated on EC2 or checked in paired Telegram.
Live Brave search still needs a configured API key. No browser automation was
enabled or tested by this fix.
