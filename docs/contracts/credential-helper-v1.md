# Credential helper v1

The host uses macOS Keychain by default on macOS. Set `CHUNSU_SECRET_HELPER` to
an absolute, regular, non-group/world-writable executable to use a different
store. Linux requires a configured helper before connecting real accounts. A
store failure never falls back to plaintext configuration or database values.
Switching Gmail/Telegram stores requires reconnecting their references; it
does not copy credentials from the previous store.

The host invokes the helper without arguments, sends one JSON object on stdin
and reads one bounded JSON object from stdout. It never logs raw helper output.
The helper is trusted host code and may use a vault-specific host environment.
Those settings are not forwarded to AI process environments.

```json
{"version":1,"operation":"get","service":"chunsu","account":"REFERENCE_ID"}
```

Operations are `get`, `set` (with `value`) and `delete`. Reply with
`{"status":"ok","value":"..."}` for get, `{"status":"ok"}` for mutation,
or `{"status":"missing"}` for an absent reference. Return nonzero on other
failures and keep diagnostics free of input/value content. Values are bounded
by the application's existing secret size. Set is followed by read-back
verification. A company vault or AWS Secrets Manager adapter can implement
this protocol without changing agent execution or SQLite.

Jira connection profiles explicitly select `macos-keychain` or
`credential-helper`; the stored service/account pair must exist in that store.
Existing `--keychain-service`/`--keychain-account` names remain reference inputs,
and `--secret-store credential-helper` selects the server adapter. Model task
packages never contain this credential reference or the credential value.

## Optional encrypted file helper

`chunsu-secret-store` implements this protocol using AES-256-GCM, random nonces
and authenticated service/account identity. Set `CHUNSU_SECRET_STORE_DIR` to a
private ciphertext directory and `CHUNSU_SECRET_KEY_FILE` to a separately
provisioned private file containing exactly 32 random bytes. Both paths are
absolute. No key is generated implicitly or stored in the ciphertext record.
The key can come from a mounted secret or a systemd credential; the caller must
give the helper a readable path under the approved host identity.

Install this helper only when needed. Preserve the master key separately from
Chun-su backups; losing it makes the encrypted credentials unreadable. An
encrypted file and an accessible key under the same OS account do not create
an additional boundary against a compromised host owner. Company vault policy,
managed workstation identity or the OS-provided credential mechanism remains
the control for that trust boundary.

Use `chunsu gmail secret-check` to write/read/delete a random non-production
marker. It returns verification status, never the marker or real tokens.
Encrypted files are named by a service/account digest. Tampered ciphertext,
wrong keys, invalid sizes and insecure key-file permissions fail closed.
