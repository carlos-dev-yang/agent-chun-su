# Saved-Mail Implementation Checkpoint

Date: 2026-09-06. Status: implementation checkpoint, not Phase 2 completion.

## Implemented

P2-01/P2-02 provide strict saved snapshots, a public guide and JSON Schema, target/reference distinctions, prior interpretations, synthetic examples, versioned immutable workgroup bundles, and pinned execution packages with SHA-256 manifests. Bodies remain behind an attempt-scoped MCP source reader.

P2-03 through P2-06 have an initial Codex adapter, bounded lookup evidence, structural/source validation, Markdown rendering, job finalization, retry/wait handling, foreground worker and protected local cancellation/queue channel. They remain in progress until an actual executor walkthrough validates the complete integration. Codex 0.153.4 is the only accepted adapter version; another version requires review. Real account selection is pending user response. No model call has been made.

## Checks actually run

- Built the application and ran focused `go vet` for affected packages.
- Parsed all three synthetic source documents.
- Offline report validation accepted successful zero-target input and classified known partial input separately; rejected unsupported references, mismatched snapshot boundaries and future source data.
- Used an intentionally unavailable executable in a temporary data root. The attempt became waiting_input, retained its package and failure metadata, and made no model call.
- Verified package file digests and that source bodies were absent from initial instructions.
- With explicitly seeded local running state, exercised the real MCP server: allowed source lookup, out-of-scope lookup and access after revocation had distinct outcomes. This was a protocol walkthrough, not an AI run.
- Codex's explicit permission-profile command probe allowed a designated marker read, rejected an unrelated marker read, and rejected writing inside the package.
- A local TCP probe reported Operation not permitted and reached no listening server. A Unix-socket probe failed and reached no listening server; the full executor/management-channel boundary still needs the real invocation walkthrough. Initial diagnostic attempts needed verbose network output and a shorter temporary Unix-socket path.

No test files or generated test suite were added. The executable, temporary evidence, private records and credentials are outside commits.

## Pending evidence and limits

P2-07 is not started. Actual model output, effective tool inventory, private-answer/control/credential denial through the real adapter, timeout/cancellation, process crash recovery, stale-result fencing, lookup exhaustion and real semantic usefulness still require validation. Permission profiles constrain command execution; they do not themselves constrain every trusted Codex facility. The adapter separately disables unrelated facilities and supplies only the scoped mail MCP server, but this complete configuration is not yet proven by a live invocation.

The prototype records process-start intent and process identity. An unidentifiable surviving process group blocks recovery instead of risking a reused PID. Reports are local artifacts; they are never treated as proof that the user read them. Unsupported attachments always remain an explicit acquisition limitation.

The small management channel was added to support cancellation during a manual attempt. Background activation, scheduling and Phase 5 completion remain separate work. No Gmail reads, mail changes or Jira implementation occurred.
