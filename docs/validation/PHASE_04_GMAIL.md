# Gmail Connector Checkpoint

Date: 2026-09-06. The selected Gmail pilot account is connected with read-only scope. After the user's authorization and actual executor verification, one bounded batch of 20 message bodies was collected and a real AI candidate report was produced. Additional mailbox pages remain; MS2 is not complete. Earlier checkpoint failures below are preserved as history.

## Current bounded live report

- The [executor walkthrough](EXECUTOR_BOUNDARY_2026-09-06.md) passed before
  setting the approved executor configuration's live disclosure flag. The actual
  adapter used the existing Codex login, CLI 0.153.4 and GPT-5.5.
- One seven-day inbox collection listed 20 messages, with no thread expansion.
  All 20 normalized bodies were complete, none had declared attachments, and
  no returned source labels contained `SPAM` or `TRASH`. A next-page token was
  preserved; no continuation was requested. This is not the whole week's inbox.
- The collector made bounded read-only list/get requests and kept credentials
  on the host. The source gateway served the immutable snapshot to the executor;
  it could not select another account, an arbitrary URL or a write operation.
  No independent before/after Gmail label comparison was run, so the read-only
  claim rests on the granted scope, invoked read paths and gateway restrictions.
- The first candidate report inspected all 20 target bodies, merged repeated
  alerts into four business items, excluded four promotional sources and kept
  one source reference-only. Structure, bidirectional mappings, acquisition gaps
  and saved artifact hashes passed validation. Operational state is `partial`
  because additional pages exist; semantic usefulness is not automatically scored.
- AI review recorded a finding about operational alerts being categorized as
  new work, repetition of excluded brands and technical status wording. The
  first output was preserved and a separate candidate rerun used the same input.
  No additional Gmail collection or active-rule adoption is implied.
- The revised run returned five items, all in the operational category, and
  excluded the same four newsletters. It kept the notification whose underlying
  message content was missing as an uncertain fifth item. Twenty successful
  lookups, matching input/manifest/artifact hashes, 17 resolving source-document
  links and owner-only file permissions were checked. Displayed states and
  timestamps are Korean/local-time views of unchanged raw values.
- Two remaining semantic issues are recorded, not erased: the model changed a
  notification-only source from reference to an uncertain item, and its item
  order still puts normal-importance alerts before high-importance items. No
  independent rubric judgments or human usefulness review exists yet. The
  comparison correctly reports `comparable: false` for that missing evaluation.
- Both reports are experiments. The unused baseline queue entry was cancelled;
  experimental results do not advance production coverage or replace production
  history. Read-only database inspection confirmed one acquisition, zero source
  coverage rows and zero enabled schedules. The active workgroup digest was
  unchanged. Local availability does not acknowledge user readership.

The [reporting baseline](../contracts/mail-pilot-reporting.md) defines category,
exclusion and source handling. A local summary links to a source document with
sender, subject, received time, source/thread IDs, disposition reasons and inert
normalized body text. Files are private and outside Git. These are collected
text records, not original EML files or downloaded attachments.

Native CGO-disabled build, focused vet of the directly affected mail, Gmail,
workgroup, executor, gateway, runner, store and CLI packages, and whitespace
checks passed. Both `report --path` and `report --sources --path` verified their
saved artifacts. No test files or broad test suite were added/run for this work.

For local retrieval, the first candidate job is `HB6F5V3RAC467ND5G7YSKBXFCQ`;
the revised candidate job is `J3G5ZHDXEZKIQIZIINNXALZA3C`. Use `report JOB_ID`
and `report JOB_ID --sources` in the existing private data root. Findings,
proposals, the comparison and all raw evidence remain linked in local records;
no account identifiers, body content or credentials are copied into this note.

Still unverified: further-page continuation, provider failure/retry scenarios,
reauthorization/revocation, long-duration refresh, independent label comparison,
calendar/attachment reads, user usefulness and unattended operation. No phase
or live-usefulness milestone is closed by this sample.

## Historical connector checkpoints

The following sections preserve the implementation, authentication failures
and eventual successful connection in order. Statements about no collection
or pending executor access describe those earlier checkpoints.

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
- The next diagnostic step at that checkpoint was user-side reauthentication of the default login Keychain, followed by one marker check after that state change. No Keychain reset, access-rule change, credential-backend change, Google OAuth request or mail read was performed during that diagnosis.

## Successful pilot connection and refresh

- The user reported `verified` from the marker check and requested verification. The agent reran the authorized connection flow; its own random-marker preflight succeeded before opening the browser. The flow then completed with exit 0, verified the selected account and exact `gmail.readonly` scope, and saved separate client-secret/refresh-token references after Keychain write/read verification.
- A new CLI process ran `gmail check` for the saved connection and exited 0. This path retrieves the credentials from Keychain, obtains an access token from the stored refresh token and calls the Gmail profile endpoint. The account matched and the command explicitly reported `message_bodies_read: false`. This verifies one real refresh and account check, not long-duration refresh reliability or token rotation/revocation.
- The saved policy is recent seven-day inbox mail, at most 20 listed messages per batch, no related-thread history, `Asia/Seoul`, manual retention and local reports. No message query, collection, queue admission, AI invocation or mailbox mutation was performed. The real-mail disclosure gate was not changed.
- Credential files, stored token values, account identifiers and private connection records remain outside the committed evidence. No implementation code changed for this checkpoint; no build or static checks were repeated. The connection flow and independent account check are the validations actually run.
- P4-02 is unblocked and remains in progress for the wider handling policy; P4-03 remains in progress because provider message reads and failure/revocation scenarios are still unverified. No phase or live-usefulness milestone is completed by authentication alone.

## Not run / remaining evidence

Provider message reads/pagination/retries, reauthorization/revocation, token rotation and long-duration refresh, actual before/after label observations, the full selected-executor boundary, real mail reports and user usefulness review remain unverified. The Keychain/account setup blocker is cleared for the selected host/account; this does not establish portability or live-report readiness. Saved-mode development, report records and local operations preparation remain independent.

No generated test suite was created. Earlier seeded records were explicitly synthetic; the later OAuth and refresh checks used the authorized pilot account. Both synthetic runtime data and private live connection records are kept out of Git.

## Final integration review checkpoint

- An offline query preview fixed a seven-day window at the declared as-of time, preserved a quoted literal operator, handled a negated year bound, and refused unsupported week syntax. No provider query was sent.
- Seeded prior local report selection retained an interpretation only inside its source/time envelope and excluded a future report. Generic feedback was not automatically treated as a human correction.
- Seeded production publication repaired deliberately missing coverage without rerunning an executor. A candidate experiment did not advance production coverage.
- Report rendering neutralized terminal control characters and escaped literal Markdown/HTML syntax. This was a synthetic presentation checkpoint, not a model-generated report.
- A declared-live envelope using a disabled-by-default disclosure setting was blocked before executor inspection or invocation. All account/source markers were synthetic. Changing the executor and restoring a backup reset the approval setting.

Code review also separated token-refresh transport/provider failures from authentication repair and bounded large `Retry-After` values. The later connection checkpoint verifies the initial live OAuth/profile path and one refresh; provider message reads, refresh failure handling and calendar-date edge cases remain unverified. A prior report is context, not proof that its interpretation was correct; human usefulness and correction policy still require review.
