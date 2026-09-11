# Phase 7 — Installation, Packaging, and Declared Compatibility

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** In progress (bounded setup dialogue only; release scope and packaging pending)
**Goal:** Make the proven local workflow easy to install and maintain for a clearly stated user and platform scope.

The user-authorized [six-service onboarding preparation](INTEGRATION_ONBOARDING.md)
provides research, bundled setup guidance and an offline preparation helper for
P7-01/P7-03. The later user-approved [guided setup delivery](../validation/SETUP_CHAT_2026-09-11.md)
adds a bounded conversation/host path for local manuals and Gmail. General packaging,
other live integrations and release acceptance remain pending; this does not complete P7-03.

**Entry conditions:** The manual feedback/live-mail milestones are complete. Service claims require P5-09. Public distribution and additional OS support remain explicit choices.

**Exit evidence:** A release candidate can be installed and operated on the declared target without source compilation, with honest executor and OS prerequisites.

**Out of scope:** Automatic public publishing, unverified Windows/Linux support claims, a hosted service, and rewriting the core solely for speculative adoption.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P7-01 — Define the first release support matrix

- **Status:** not_started
- **Depends on:** P4-09; P5-09 if service support is included; D09.
- **Work:** Choose supported OS/CPU/minimum-version combinations, manual versus service features, executor versions, secret-store behavior, release channel, and update policy. Start with verified targets rather than promising every platform supported by Go.
- **Deliverable:** A release-scope decision and support matrix.
- **Acceptance evidence:** Every supported feature has an intended verification environment. Additional platforms are explicitly conditional, and signing/publication requirements are separated from local packaging.

### P7-02 — Produce the target release binary and assets

- **Status:** not_started
- **Depends on:** P7-01.
- **Work:** Build the core with pinned dependencies and version metadata; bundle required public assets and migration information. Record checksums, third-party notices, native dependencies, and build inputs. Keep real data and private expectations out of the bundle.
- **Deliverable:** A local release candidate with manifest and checksums.
- **Acceptance evidence:** The candidate runs without a Go compiler or SQLite CLI. The first executor's separate prerequisites are accurately detected and documented; a Go binary is not claimed to remove those dependencies.

### P7-03 — Provide a terminal-first install and setup path

- **Status:** in_progress (guided setup subset implemented; general installer/package pending)
- **Depends on:** P7-02, P1-02, P1-06.
- **Work:** Implement or package a simple user-local install/update/remove path and clear setup/help. Verify paths independently of the development shell. Preserve existing configuration, active controls, reports, and secrets.
- **Deliverable:** Installer/package assets and a concise user guide.
- **Acceptance evidence:** A fresh environment reaches a saved-input report through documented steps. Removal of the executable does not silently delete user data; account setup requirements are stated.

### P7-04 — Verify update, compatibility, and recovery behavior

- **Status:** not_started
- **Depends on:** P7-03, P5-08.
- **Work:** Check upgrade compatibility for DB schema, stored artifacts, control versions, and executor prerequisites. Back up appropriately and detect unsupported downgrades instead of resetting data. Validate service replacement if included.
- **Deliverable:** An upgrade/rollback runbook and evidence.
- **Acceptance evidence:** A compatible update preserves data. An older binary encountering a newer incompatible schema stops clearly. Recovery is demonstrated in a separate data root without overwriting the user's active environment.

### P7-05 — Implement additional OS adapters only when selected

- **Status:** not_started
- **Depends on:** P7-01; selected platform portion of D09.
- **Work:** For each newly selected target, implement its service/secret/path/process/isolation behavior and use the same core contracts. If no additional target is selected, record this task as deferred and retain a narrower support matrix.
- **Deliverable:** Target-specific adapters and verification records, or an explicit deferral.
- **Acceptance evidence:** A successful cross-build alone is not support evidence. Each claimed target passes the relevant install/run/cancel/secret/service checks on that environment.

### P7-06 — Close the release-readiness checkpoint

- **Status:** not_started
- **Depends on:** P7-04; P7-05 only for selected additional targets.
- **Work:** Assemble release notes, known limitations, install evidence, and resource measurements separating core from executor. Prepare the exact release artifact for review. Publish or push only if the user authorizes that concrete action.
- **Deliverable:** The MS5 release-readiness note and a reviewable local release candidate.
- **Acceptance evidence:** Support claims match the performed checks. A candidate prepared locally is not reported as publicly released, and deferred platforms remain clearly unsupported.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.
