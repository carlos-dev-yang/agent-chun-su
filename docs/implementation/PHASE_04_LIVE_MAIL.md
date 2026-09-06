# Phase 4 — Live Read-Only Mail

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** Pilot account connected; one 20-message live collection and candidate report verified; acquisition is partial and human usefulness review remains pending

**Goal:** Validate the useful mail feedback loop against one explicitly scoped real account without modifying remote mail or calendars.

**Entry conditions:** P3-07 is complete. The first provider, account, data boundaries, and permitted reading scope are selected before dependent integration work.

**Exit evidence:** A small live pilot produces source-backed reports, preserves partial/failure evidence, demonstrates the read-only boundary, and records actual user value.

**Out of scope:** Additional providers, automatic external report messages, full-service search, mailbox mutations, calendar writes, and unattended operation.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P4-01 — Define the first live connection and data policy

- **Status:** in_progress
- **Depends on:** P3-07; D05/D06.
- **Work:** Prepare the concrete provider/account/folder-or-query/time scope, allowed body/attachment/history access, related-service lookup policy, disclosure boundary, retention/deletion, and local report destination. Verify current official provider documentation for the chosen integration.
- **Deliverable:** A reviewed live-mail connection and data-policy decision.
- **Acceptance evidence:** No token is requested in chat. Supported reading/authentication behavior is distinguished from assumptions, and unresolved scope does not become broad access by default.

### P4-02 — Implement host secret storage and live-data handling

- **Status:** in_progress
- **Depends on:** P4-01.
- **Work:** Implement the selected Keychain bridge and secret-reference lifecycle, plus the initial retention/deletion and diagnostic-redaction rules needed before live data enters storage. Separate non-secret bindings from credential values; avoid secrets in arguments, general config, packages, and logs.
- **Deliverable:** The secret-store adapter and effective live-data storage/redaction policy.
- **Acceptance evidence:** Non-production material verifies secure input/reference lookup, unavailable or locked storage, redaction, and scoped deletion behavior. Native build requirements and same-user access limits are documented; no plaintext fallback occurs silently.

### P4-03 — Implement provider authentication and bounded reads

- **Status:** in_progress
- **Depends on:** P4-02.
- **Work:** Implement the selected OAuth or token flow, account identity check, scoped list/get operations, pagination, rate limits, authentication errors, and connection revocation. Use official APIs; classify provider retry advice with named limits.
- **Deliverable:** One mail provider connector and terminal connection setup/check commands.
- **Acceptance evidence:** Authorization completes only for the selected account/scope. Empty successful listing, partial pages, expired authentication, and provider failure are distinguishable. No write endpoint is used to validate credentials.

### P4-04 — Bind live tools to attempts and host credentials

- **Status:** in_progress
- **Depends on:** P4-03, P2-04.
- **Work:** Expose only reviewed domain reads through the gateway. Bind connection and scope on the host, validate requested identifiers, avoid forwarding authentication to arbitrary source URLs, and revoke access when the job or connection is cancelled.
- **Deliverable:** Live tool bindings using the same observable tool contract as saved-input operation.
- **Acceptance evidence:** A request cannot select another account or expand scope by supplying a different connection identifier. Tool evidence contains content/error metadata without credential values.

### P4-05 — Normalize messages and explicitly supported attachments

- **Status:** complete
- **Depends on:** P4-03; attachment portion of D05/D06.
- **Work:** Handle MIME, encoding, HTML-to-text, quoted history, source identity, and supported attachment extraction. Keep original provenance, explicit truncation/unsupported status, named limits, and safe extraction. Do not fetch tracking images or execute attachments.
- **Deliverable:** Provider normalization and a deliberately limited attachment path.
- **Acceptance evidence:** Representative supported and unavailable input forms retain evidence and truthful status. Unsupported attachments produce partial/unknown behavior rather than invented content.

### P4-06 — Persist acquisition progress separately from processing

- **Status:** in_progress
- **Depends on:** P4-04, P4-05.
- **Work:** Persist collected snapshots and progress so a cursor advance does not lose unprocessed items. Handle duplicate pages, late arrivals, interrupted retrieval, expired cursors, and retry snapshots. Link history through source evidence and corrections, not unverified prior AI prose.
- **Deliverable:** Durable acquisition progress, source mapping, and reviewed history updates.
- **Acceptance evidence:** An interruption after collection but before report creation leaves the collected material discoverable. A source refresh creates a new input version instead of altering an earlier attempt's evidence.

### P4-07 — Verify the live boundary before an AI report run

- **Status:** in_progress
- **Depends on:** P4-06, P2-03.
- **Work:** Check actual connector routes, granted scopes, executor restrictions, secret handling, revocation, and the absence of implicit read-state or other remote changes using the approved small scope. Verify persisted data handling before sending live content to the executor.
- **Deliverable:** A concrete live-boundary verification record.
- **Acceptance evidence:** Actual reads and available before/after source-state evidence support the read-only claim. Unverified behavior is a blocker for the affected capability; no destructive or write call is used as a test.

### P4-08 — Run live reports with truthful partial and delivery states

- **Status:** in_progress
- **Depends on:** P4-07.
- **Work:** Connect live acquisition to the existing package/execute/validate/local-report path. Preserve known missing content, no-new-mail behavior, prior interpretation corrections, and explicit waits. Distinguish generated/validated/locally available from user acknowledgment.
- **Deliverable:** A manual live mail-review command and traceable local report bundle.
- **Acceptance evidence:** Normal input, successful empty input, known partial input, and retrieval failure cannot all be presented as an empty successful report. Local file creation is not falsely recorded as confirmed user readership.

### P4-09 — Evaluate the live pilot and close MS2

- **Status:** not_started
- **Depends on:** P4-08, P3-07; D10.
- **Work:** Collect human judgments on representative authorized live reports, including important omissions, incorrect urgency, unwanted exclusions, and correction burden. Use the existing feedback loop for observed issues and retain failed runs in the record.
- **Deliverable:** The MS2 evidence note and prioritized fixes or explicit limitations.
- **Acceptance evidence:** Actual user review supports the usefulness claim. Synthetic evaluations remain separately labeled, and readiness for scheduled operation is assessed without claiming full production reliability.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.

Current evidence: [Gmail connector checkpoint](../validation/PHASE_04_GMAIL.md). P4-05 completion covers the declared text-only normalization subset, not attachment extraction or live-provider verification.

The user explicitly authorized the bounded live experiment after the actual
synthetic executor boundary check, before Phase 3 human acceptance. This trial
advances P4-01 through P4-08 evidence without closing MS1/MS2. All 20 collected
targets were available and inspected; additional mailbox pages remain. No
additional page, related thread, linked service or calendar was read. User
usefulness, before/after provider-state comparison and failure/revocation cases
remain separate evidence.
