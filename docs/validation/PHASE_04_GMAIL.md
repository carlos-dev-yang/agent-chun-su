# Gmail Connector Checkpoint

Date: 2026-09-06. No Gmail account has been connected and no real mail or calendar data has been read. MS2 is not complete.

## Implemented

The terminal connector supports Desktop app OAuth with state/PKCE and a temporary loopback callback, strict read-only scope/account checks, Keychain references, bounded list/get/thread reads, MIME normalization, collection resume/page continuation, acquisition-to-job deduplication and a manual review command. Token-bearing requests use provider-owned HTTPS routes and reject redirects. Access tokens remain in memory; refresh/client secrets use separate Keychain entries and never enter configuration, packages or ordinary diagnostics.

Schema 3 adds acquisition checkpoints and per-source report coverage. A checkpoint is an immutable file published before its database reference. Collection, pending source retrieval, next-page continuation, job admission and reported source coverage are separate facts. The collector preserves failures and current gaps separately. Only a validated locally available result can advance coverage; coverage is not a Gmail read-state change or human acknowledgment. Snapshot-origin and connection-policy checks bind the source gateway to the selected scope.

Text normalization supports MIME charsets, plain-text preference, HTML text, bounded UTF-8 output, explicit missing text and unsupported attachments. It neither loads tracking resources nor separately downloads/executes attachments. The initial retention policy is manual; records are not silently deleted. The first account, query and actual executor choice remain pending user input; see [pilot setup](../setup/GMAIL_PILOT.md).

## Checks actually run

- Built the CLI and ran focused vet on changed connector and directly affected core packages.
- Synthetic saved Gmail API messages verified charset conversion, plain-text alternative preference, unsupported attachment disclosure, HTML script/style removal, future-source rejection, UTF-8-safe truncation and unavailable status for invalid base64.
- A manually seeded synthetic acquisition verified that repeated queue admission returns the same job and repairs a deliberately cleared acquisition/job link. This is a persistence walkthrough, not a provider pagination test.
- Keychain validation attempted a random non-production marker. The native bridge and a narrowly scoped diagnostic attempt both failed to create the marker. macOS reported authentication error `-25293` (native process exit 51), with the message that the user name or passphrase was incorrect. Lookup/deletion of that diagnostic marker returned item-not-found. No Gmail token was involved.

The same Keychain operation will not be retried until the user confirms the local Keychain state has changed. No plaintext fallback was added. The user has been asked to check the login Keychain and to provide the pilot account/scope and local Desktop app client-file path, without sharing secrets in chat.

## Not run / remaining evidence

Successful Keychain storage, live OAuth/account verification, refresh/revocation, provider pagination/retries, actual before/after label observations, the full selected-executor boundary, real mail reports and user usefulness review remain unverified. Live connection handling is blocked on the Keychain/account prerequisites. Saved-mode development, report records and local operations preparation remain independent.

No generated test suite was created. All local records used in these checks were explicitly synthetic and kept out of Git.

## Final integration review checkpoint

- An offline query preview fixed a seven-day window at the declared as-of time, preserved a quoted literal operator, handled a negated year bound, and refused unsupported week syntax. No provider query was sent.
- Seeded prior local report selection retained an interpretation only inside its source/time envelope and excluded a future report. Generic feedback was not automatically treated as a human correction.
- Seeded production publication repaired deliberately missing coverage without rerunning an executor. A candidate experiment did not advance production coverage.
- Report rendering neutralized terminal control characters and escaped literal Markdown/HTML syntax. This was a synthetic presentation checkpoint, not a model-generated report.
- A declared-live envelope using a disabled-by-default disclosure setting was blocked before executor inspection or invocation. All account/source markers were synthetic. Changing the executor and restoring a backup reset the approval setting.

Code review also separated token-refresh transport/provider failures from authentication repair and bounded large `Retry-After` values. Live HTTP, refresh, calendar-date edge cases and OAuth behavior remain unverified. A prior report is context, not proof that its interpretation was correct; human usefulness and correction policy still require review.
