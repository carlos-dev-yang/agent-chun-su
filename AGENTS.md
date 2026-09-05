# Codex Instructions

## Planning context

- Go + SQLite is the selected implementation stack.
- Read `IMPLEMENTATION_PLAN.md` and the relevant file in `docs/implementation/` before starting a planned task.
- The plan defines work units and dependencies; it does not mean every phase is already authorized for execution or complete.
- Preserve the current principles: mail first, result feedback before extensive infrastructure, independent result evaluation and optimization, and human ownership of active controls.

## Implementation

- Do not create test code unless the user explicitly asks for it; run existing tests when relevant.
- Do not hard-code environment-specific values, credentials, endpoints, file paths, magic numbers, or duplicated business rules. Use configuration, environment variables, named constants, parameters, or derived values.
- Work in the current checkout. Do not create a worktree unless the user explicitly asks or active instructions require one.

## Validation

- Validate changed behavior and directly affected integration points in coherent batches.
- Do not run broad or full-project validation unless the user asks or the change is genuinely cross-cutting.

## Decision Boundaries

- Use an internal execution plan for routine work; do not require Plan mode merely because a task has multiple steps.
- Before editing high-risk work whose direction is not already established, present the intended direction, change scope, and completion evidence for user confirmation. Treat unresolved architecture or product choices, external interfaces, schema or data migrations, permissions, deployments, and irreversible effects as high risk.
- Do not infer new product direction, change an external contract, or perform an irreversible action without the user's decision.
- If the same approach fails twice for the same cause, do not repeat it or widen scope; stop and report the evidence and required decision.
- Before claiming completion, separate checks actually run from checks not run, and report remaining risks or required decisions.
- Use subagents only for bounded, independent investigation or review; do not have multiple agents edit the same files concurrently.

## Git Discipline

- The user has authorized Git initialization and local commits as work is completed.
- Commit validated work in bounded task or phase increments. Include the phase/task ID and concrete result in the subject; include relevant validation and limitations in the body when helpful.
- Stage only the intended work-unit files and inspect the staged changes before committing.
- Update task completion status and evidence accurately; committing one task does not complete the whole phase.
- Preserve existing user work. Do not commit credentials, private runtime data, databases, or local build outputs.
- Do not configure a remote, push, or publish merely because local commits are authorized.
