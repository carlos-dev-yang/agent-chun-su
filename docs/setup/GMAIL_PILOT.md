# Gmail Pilot Setup

Status: connector implementation in progress. No Gmail account has been connected or read.

## Information needed for the first pilot

- The exact Gmail account to use.
- A Google OAuth **Desktop app** client JSON file available locally. Provide its file path, not token or secret values in chat.
- A reviewed query and batch size. Proposed starting scope: `in:inbox newer_than:7d`, at most 20 listed messages per batch, body review only, no separate attachment downloads and no calendar access. Related-thread history is opt-in and bounded by a declared age/message limit.
- The selected executor account. The existing Codex login is available, but its use is awaiting the user's response.

Google requires an enabled Gmail API and an OAuth client for the installed-app flow. If a client is not already available, create a project/client in Google Cloud, configure the consent audience and permitted test account, and download a Desktop app client file. The initial account consent uses the system browser; subsequent operation uses the terminal. See [Google's installed-app authorization guide](https://developers.google.com/identity/protocols/oauth2/native-app) and [Gmail Go setup prerequisites](https://developers.google.com/workspace/gmail/api/quickstart/go).

## Concrete connection behavior

The connector opens a temporary loopback callback, generates state and PKCE values, requests only `gmail.readonly`, checks the returned scope and the authenticated account, then stores the refresh token and desktop client secret in separate macOS Keychain entries. Access tokens remain in memory. General configuration stores opaque secret references. An unsupported or unavailable Keychain has no plaintext fallback.

The OAuth permission covers Gmail read access; query and batch restrictions are enforced by the host connector. The source gateway exposes only collected message IDs from one attempt. It exposes no credential, arbitrary URL, attachment-download, send, modify, delete, label or calendar action. Google's [scope documentation](https://developers.google.com/workspace/gmail/api/auth/scopes) distinguishes read access from modification/sending privileges.

The pilot uses bounded message listing and explicitly resumable pages. Each collected batch becomes a preserved snapshot before analysis is queued. An incomplete page, missing message, unsupported attachment, expired cursor or authentication failure remains visible. Page continuation and report completion are separate records; successful collection of zero messages is a distinct outcome. See the provider's [message-list contract](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/list).

## Local handling and boundaries

Data stays in the private application root: snapshots, lookup evidence, reports, evaluations and provenance. The selected AI executor receives only the scoped package and source tool results; model processing occurs under that executor's existing account. Reports are local files, with user readership unknown. Retention starts as **manual**: there is no automatic deletion before an explicit retention action. Disconnecting credentials preserves existing local reports and evidence; deleting local records is a separate operation.

The setup request does not authorize Jira, mailbox changes, external report messages, background activation, or a broad mailbox scan. Live reporting still requires completion of the real executor boundary checks and the initial feedback review. Current synthetic/structural checks are not a live pilot success claim.
