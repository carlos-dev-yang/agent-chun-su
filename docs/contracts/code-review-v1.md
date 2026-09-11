# Code review v1

The owner selects a local repository, base and head commit, and exact file paths.
The host resolves both commits and reads their regular Git blobs and diff using
literal paths, no external diff/textconv and no allowed fetch protocols. It
does not check out revisions, run repository commands, apply patches or traverse
submodules/symlinks. Missing local objects and byte-budget overflow fail capture.
Uncommitted and unselected files stay outside the snapshot. The original
repository is not mounted in the AI execution environment.

The immutable input stores the repository label, resolved commit IDs, capture
time, synthetic marker, and per-file before/after/diff plus content digests. The
source ID is the digest of its repository-relative path. Imported saved inputs
are owner-provided snapshots; their commit strings are not independent proof
of a remote Git server. Git capture observes the actual local object contents.

The executor receives only the index and its pinned Skill/schema, then obtains
the selected source bodies through `code_source_get`. The gateway accepts only
exact source IDs, checks the active attempt and current disclosure permission,
and preserves actual lookup evidence. It accepts no paths or commands.
Non-synthetic code requires the selected route's `live_code_approved` permission.
Changing the route or restoring a backup revokes it. Independent review uses
its own route and the same disclosure requirement; reception sees job summaries
and can display a report locally without sending its body to the reception AI.

The host validates result schema, repository/revision identity, exact source
coverage, actual source lookups and finding line ranges. Quoted evidence must be
present in those lines. A host-validated review is not a semantic correctness
score, a passing build/test, or authorization to make a change. The presentation
states which checks were performed and that repository execution was absent.

The common feedback system accepts `code_snapshot` cases, pinned evaluator
Skills/rubrics, optional selected-result reviews, analyzer/optimizer criteria,
proposals, candidate comparisons and human-owned release policies. The public
example is marked validation, not human acceptance. No automatic AI evaluation,
control activation, patch, commit, merge, deployment or publication is added.
