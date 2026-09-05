# Phase 9 — Bounded Development-Work Pilot

[Master plan](../../IMPLEMENTATION_PLAN.md)

**Status:** Not started  
**Goal:** Extend the validated framework to one explicitly scoped repository task after mail and Jira have demonstrated the common model.

**Entry conditions:** P4-09 and P6-07 establish mail usefulness and workgroup reuse. D12 selects a concrete development task and its authority; the planning roadmap does not authorize arbitrary repository edits.

**Exit evidence:** One bounded development request produces a reviewable change and appropriate validation, traceable through the same execution/evidence/feedback records.

**Out of scope:** Unrestricted coding autonomy, arbitrary repository access, automatic merge/deploy/publish, and a new multi-agent platform.

All implementation paths and commands below are planned deliverables. Acceptance
scenarios are not completed checks or authorization to create test code. Follow
the work-unit and validation rules in the master plan.

## Work units

### P9-01 — Select the first development task and authority

- **Status:** not_started
- **Depends on:** P4-09, P6-07; D12.
- **Work:** Choose one suitable task, repository, allowed paths/actions, required existing checks, dependency/network access, output requirements, and external-effect boundaries. Follow that repository's active instructions and current-checkout/worktree rules.
- **Deliverable:** A concrete development workgroup brief and permission decision.
- **Acceptance evidence:** The allowed change and completion evidence are reviewable before execution. Unrequested test creation, repository migration, merge, or deployment is not included by assumption.

### P9-02 — Prepare a scoped development execution environment

- **Status:** not_started
- **Depends on:** P9-01, P2-03.
- **Work:** Bind source inputs and allowed tools to the task; restrict control/secret/unrelated-project access and constrain filesystem/network effects. Record repository state and any pre-existing changes without overwriting them.
- **Deliverable:** A development adapter configuration using the existing executor boundary.
- **Acceptance evidence:** The executor can access the intended repository scope and cannot use development permissions to modify the framework's active controls. Workspace preparation follows explicit user/repository instructions.

### P9-03 — Define development outputs and validation contracts

- **Status:** not_started
- **Depends on:** P9-01; review the workgroup contract extension.
- **Work:** Specify the patch/change summary, affected paths, command/check evidence, unresolved issues, and task-specific completion. Use relevant existing build/tests; create new test code only if explicitly requested.
- **Deliverable:** A development guide, result contract, and validation checklist.
- **Acceptance evidence:** A claim of completion requires relevant observed results. A passing build does not substitute for task-specific behavior, and missing checks remain visible.

### P9-04 — Execute one bounded change and collect evidence

- **Status:** not_started
- **Depends on:** P9-02, P9-03.
- **Work:** Run the authorized task, review the resulting diff, execute the relevant checks, and preserve failures and existing user work. Apply and commit only within the selected authority and current repository instructions.
- **Deliverable:** A reviewable change plus execution, diff, and validation evidence.
- **Acceptance evidence:** Only intended files/actions are included. The outcome reports checks actually run and not run; no automatic merge/deployment is bundled with local completion.

### P9-05 — Evaluate the pilot and decide further development scope

- **Status:** not_started
- **Depends on:** P9-04, P3-02.
- **Work:** Review correctness, important omissions, review burden, and which common controls helped or failed. Use the existing proposal loop for improvements and explicitly choose whether to expand.
- **Deliverable:** A development-pilot completion note and a narrowly scoped next proposal.
- **Acceptance evidence:** The one-task outcome is not generalized to unrestricted development capability. Mail/Jira still share the same core records, and further permissions or workflows remain deliberate decisions.

## Completion record

Record task IDs, changed artifacts, checks actually run, checks not run,
remaining risks or decisions, and newly ready work in the phase completion note.
A successful check for one scenario does not establish every phase capability.
