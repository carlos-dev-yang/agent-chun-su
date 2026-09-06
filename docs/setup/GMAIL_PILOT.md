# Gmail Pilot Setup

Status: the selected pilot account is connected with `gmail.readonly`. Credential storage, refresh and one 20-message collection passed on 2026-09-06. Two real AI candidate reports used that same input after the restricted executor walkthrough. Additional pages remain; no calendar or related-service reads were performed. Human usefulness remains unreviewed.

## Information needed for the first pilot

- The exact Gmail account to use (supplied for the current pilot; keep account-specific setup outside Git).
- A Google OAuth **Desktop app** client JSON file available locally (supplied and checked for the current pilot). Provide its file path, not token or secret values in chat. The local file is restricted to owner read/write and excluded from Git.
- A reviewed query and batch size. The connected pilot policy is `in:inbox newer_than:7d`, at most 20 listed messages per batch, body review only, related-thread history disabled, manual retention and `Asia/Seoul`. No separate attachment downloads or calendar access. Related-thread history is opt-in and bounded by a declared age/message limit. One actual batch has been collected; a connection check by itself still reads no message bodies.
- The selected executor account. The user authorized the existing Codex login for the bounded trial after access verification. CLI 0.153.4 and GPT-5.5 are the tested combination, recorded in the [executor evidence](../validation/EXECUTOR_BOUNDARY_2026-09-06.md).

Google requires an enabled Gmail API and an OAuth client for the installed-app flow. If a client is not already available, create a project/client in Google Cloud, configure the consent audience and permitted test account, and download a Desktop app client file. The initial account consent uses the system browser; subsequent operation uses the terminal. See [Google's installed-app authorization guide](https://developers.google.com/identity/protocols/oauth2/native-app) and [Gmail Go setup prerequisites](https://developers.google.com/workspace/gmail/api/quickstart/go).

## Keychain authentication failure before sign-in

If the storage preflight reports `errSecAuthFailed (-25293)`, Google sign-in has not started. An unlocked Keychain status alone does not verify that a credential can be stored. Earlier pilot attempts reproduced this failure both through Chun-su and through Apple's native Keychain API. The user subsequently reported a verified marker check, and connection storage plus a fresh-process account check now succeed. The earlier failure's exact macOS cause was not established; the procedure below remains troubleshooting guidance if it recurs.

After confirming that the default Keychain is the intended login Keychain, the user can reauthenticate it in their own Terminal. Run these commands one at a time; proceed to unlock only if the lock command succeeds:

```sh
security lock-keychain
security unlock-keychain
```

This temporarily locks the default Keychain; other applications may need it unlocked again. Enter its password only at the local password prompt. The command uses an interactive password prompt when `-p` is omitted, as shown in [Apple's unlock implementation](https://github.com/apple-oss-distributions/SecurityTool/blob/main/keychain_unlock.c). This is a diagnostic reauthentication step, not a verified repair for every cause of `-25293`.

After a successful unlock, run `bin/chunsu gmail keychain-check` from the checkout once. A successful result reports `keychain: verified` and `deleted: true`. Only then retry the connection command. If unlock or marker storage fails, retain the new error and stop repeating the same attempt; do not reset/delete the Keychain or paste its password into chat. The preflight has no plaintext fallback.

## Concrete connection behavior

The connector opens a temporary loopback callback, generates state and PKCE values, requests only `gmail.readonly`, checks the returned scope and the authenticated account, then stores the refresh token and desktop client secret in separate macOS Keychain entries. Access tokens remain in memory. General configuration stores opaque secret references. An unsupported or unavailable Keychain has no plaintext fallback.

The OAuth permission covers Gmail read access; query and batch restrictions are enforced by the host connector. The source gateway exposes only collected message IDs from one attempt. It exposes no credential, arbitrary URL, attachment-download, send, modify, delete, label or calendar action. Google's [scope documentation](https://developers.google.com/workspace/gmail/api/auth/scopes) distinguishes read access from modification/sending privileges.

The pilot uses bounded message listing and explicitly resumable pages. Each collected batch becomes a preserved snapshot before analysis is queued. An incomplete page, missing message, unsupported attachment, expired cursor or authentication failure remains visible. Page continuation and report completion are separate records; successful collection of zero messages is a distinct outcome. See the provider's [message-list contract](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/list).

Preview a query without authentication using `gmail query 'in:inbox newer_than:7d' --as-of TIMESTAMP --timezone IANA_ZONE`. Chun-su resolves `newer_than`/`older_than` day/month/year counts as calendar units in that report zone and persists epoch bounds once per acquisition chain. Quoted literal text is retained; unsupported relative syntax is rejected. This is the framework's explicit time policy, not a claim that every Gmail UI relative-date edge case is identical. Google documents [relative operators](https://support.google.com/mail/answer/7190?hl=en) and [epoch-second API date bounds](https://developers.google.com/workspace/gmail/api/guides/filtering). The actual API query and pagination behavior still require the pilot.

Earlier local report interpretations are included only when all their source IDs are already in the current envelope and their as-of time is not in the future. `gmail history SNAPSHOT.json` previews this local selection without account access. It does not expand mailbox scope. Saved snapshots can explicitly carry human corrections; generic feedback findings are not silently promoted into operational facts. `changes_and_open` remains limited to the available envelope, not a complete task list across the account.

Real Gmail-to-executor disclosure requires `executor.live_mail_approved=true` after the actual synthetic boundary walkthrough and human authorization. The default is false, and executor changes/restores reset it. The selected host/configuration has now passed the documented real invocation, scoped-source, private-file, network and MCP checks. Use a private data root outside macOS temporary storage; the adapter refuses roots inside the observed temporary-directory write exception. Different models, versions or environments are not covered by that result.

## Local handling and boundaries

Data stays in the private application root: snapshots, lookup evidence, reports, evaluations and provenance. The selected AI executor receives only the scoped package and source tool results; model processing occurs under that executor's existing account. Reports are local files, with user readership unknown. Retention starts as **manual**: there is no automatic deletion before an explicit retention action. Disconnecting credentials preserves existing local reports and evidence; deleting local records is a separate operation.

The setup request does not authorize Jira, mailbox changes, external report messages, background activation, or a broad mailbox scan. The current user-authorized pilot deliberately trials a candidate before human feedback-loop acceptance. Its first and revised reports, findings and comparison remain separate records. Review usefulness before adopting the candidate or claiming MS1/MS2; generating a structurally valid local report is not a personal quality judgment.
