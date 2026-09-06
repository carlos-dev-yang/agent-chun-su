# Gmail Connector Checkpoint

Date: 2026-09-06. No Gmail account has been connected and no real mail or calendar data has been read. MS2 is not complete.

## Implemented

The terminal connector supports Desktop app OAuth with state/PKCE and a temporary loopback callback, strict read-only scope/account checks, Keychain references, bounded list/get/thread reads, MIME normalization, collection resume/page continuation, acquisition-to-job deduplication and a manual review command. Token-bearing requests use provider-owned HTTPS routes and reject redirects. Access tokens remain in memory; refresh/client secrets use separate Keychain entries and never enter configuration, packages or ordinary diagnostics.

Schema 3 adds acquisition checkpoints and per-source report coverage. A checkpoint is an immutable file published before its database reference. Collection, pending source retrieval, next-page continuation, job admission and reported source coverage are separate facts. The collector preserves failures and current gaps separately. Only a validated locally available result can advance coverage; coverage is not a Gmail read-state change or human acknowledgment. Snapshot-origin and connection-policy checks bind the source gateway to the selected scope.

Text normalization supports MIME charsets, plain-text preference, HTML text, bounded UTF-8 output, explicit missing text and unsupported attachments. It neither loads tracking resources nor separately downloads/executes attachments. The initial retention policy is manual; records are not silently deleted. The pilot account and client file have now been supplied; the actual executor choice and live execution boundary remain pending. See [pilot setup](../setup/GMAIL_PILOT.md).

## Checks actually run

- Built the CLI and ran focused vet on changed connector and directly affected core packages.
- Synthetic saved Gmail API messages verified charset conversion, plain-text alternative preference, unsupported attachment disclosure, HTML script/style removal, future-source rejection, UTF-8-safe truncation and unavailable status for invalid base64.
- A manually seeded synthetic acquisition verified that repeated queue admission returns the same job and repairs a deliberately cleared acquisition/job link. This is a persistence walkthrough, not a provider pagination test.
- Keychain validation attempted a random non-production marker. The native bridge and a narrowly scoped diagnostic attempt both failed to create the marker. macOS reported authentication error `-25293` (native process exit 51), with the message that the user name or passphrase was incorrect. Lookup/deletion of that diagnostic marker returned item-not-found. No Gmail token was involved.

After the initial repeated failure, retries were paused and the user was asked about the login Keychain state without sharing secrets in chat. The pilot account and local Desktop app client-file path were subsequently supplied. The explicit user-authorized retry below supersedes that earlier pause; no plaintext fallback was added.

## Pilot preparation follow-up

- Validated the supplied JSON as a Desktop client with the required fields without printing credential values. Restricted the regular local file to owner read/write and excluded it through the repository-local Git exclude file, preserving the user's separate ignore-file edit. No credential file was staged.
- Initialized the default private application root. Readiness inspection reported valid configuration, schema 4 and SQLite 3.53.4; the executor remains unconfigured and the connection list is empty.
- A read-only native `SecKeychainGetStatus` query returned success and status bits 7: unlocked, readable and writable, as defined by the installed SDK header. This is a status observation, not a successful write/read/delete check or evidence that the earlier authentication error is resolved. No additional Keychain write was attempted during that preparation step.
- OAuth consent, token storage, account verification and message collection were not run. File shape does not verify Google API activation, the test-user list or granted scopes. Account-specific values remain outside the committed documentation.

## User-authorized authentication retry

- The user reported no login/Keychain state change and explicitly requested another authentication attempt. The CLI's random-marker `gmail keychain-check` exited 1 with `macOS Keychain is unavailable or locked; no plaintext fallback is used`. The probe did not reach successful storage verification, and no Google OAuth flow or mail read was started.
- The latest connector error is generic. The native authentication code recorded for the earlier checks cannot be assumed to be newly reproduced by this output. An unlocked status alone does not identify or resolve the storage failure.
- A comparison in the foreground Terminal app was attempted, but the computer-use tool denied control of Terminal for safety reasons before a command was entered. This tool restriction does not explain the separate Keychain failure. The user subsequently reported the same generic error after running the connection command themselves.
- No authentication settings, Keychain access rules or plaintext-storage fallback were changed. Further diagnosis needs evidence beyond another identical automated probe.

## Native failure diagnosis and error reporting

- Replaced the generic unavailable/locked error with the failed operation, native exit status and an allowlisted macOS error name/code. Only exact known diagnostic messages with matching exit status are classified; raw native output is withheld because interactive mode can echo secret-bearing input. Missing items are distinguished from missing Keychains. Cancellation and diagnostic-marker cleanup failures remain visible.
- Connection errors now identify whether credential storage failed before Google sign-in or while saving the returned credentials. The preflight still runs before opening the browser.
- The rebuilt CLI's random-marker check reported `Keychain store failed`, `security exit 51`, and `errSecAuthFailed (-25293)`. A separate direct `SecKeychainAddGenericPassword` call, using only a random non-production marker, also returned `-25293` before a successful write. This reproduces the failure without the command-line bridge. It does not establish the underlying macOS cause or prove that the user supplied an incorrect password.
- Formatting, a native CGO-disabled CLI build and focused vet of `internal/secrets`, `internal/gmail` and `internal/cli` passed. Only the native authentication-failure path and missing-marker cleanup were exercised; successful storage and other native error cases remain unverified. No generated test code was added.
- The next diagnostic step is a user-side reauthentication of the default login Keychain, followed by one marker check after that state change. No Keychain reset, access-rule change, credential-backend change, Google OAuth request or mail read was performed.

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
