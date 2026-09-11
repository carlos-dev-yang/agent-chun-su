# Review saved results when needed

Run saved mail/Jira work normally and retain its result. No evaluator is required
for ordinary execution. Use an explicit data home for the following commands.
Replace uppercase IDs with the records returned by earlier commands.

```sh
chunsu feedback import case my-reviewed-case.json
chunsu feedback import rubric my-rubric.json
chunsu feedback pin-evaluator evaluation-skills/golden-evaluation/SKILL.md --workgroup mail-review
chunsu review evaluate JOB_ID --case CASE_ID --rubric RUBRIC_ID --evaluator-skill SKILL_ID
```

The case must match the saved job input. Example cases are validation drafts,
not evidence that a person accepted the result. The review returns request,
execution and, only after host validation, evaluation IDs. Inspect records with
`feedback show ID`; failed reviews also preserve their request/execution IDs.

For accumulated-result analysis, pin your criteria explicitly:

```sh
chunsu review pin evaluation-skills/result-analysis/SKILL.md --name result-analysis --workgroup mail-review --purpose analyzer --reviewer 'criteria author' --review-status draft
chunsu review analyze JOB_A JOB_B --criteria CRITERIA_ID --question 'Which omissions are supported by these selected results?'
```

Use `--purpose optimizer` when pinning criteria intended to suggest candidate
changes. The output must state hypotheses and validation requirements. It does
not modify or adopt a Skill. Select fewer jobs when the configured input budget
is exceeded. One controller owns SQLite; a foreground review holds the same
write lock as other management operations and should be run when that writer
is available.

For a candidate comparison and readiness review:

```sh
chunsu feedback compare BASELINE_JOB CANDIDATE_JOB --baseline-evaluation BASELINE_EVALUATION --candidate-evaluation CANDIDATE_EVALUATION
chunsu feedback import release_policy my-release-policy.json
chunsu feedback import check my-check-result.json
chunsu workgroup assess PROPOSAL_ID --comparison COMPARISON_ID --policy POLICY_ID --check-result CHECK_ID
```

A release policy uses version 1 and includes `name`, `workgroup`, `review_status`,
`reviewer`, `required_criteria`, `allowed_outcomes`, `allowed_job_statuses`,
`require_reviewed_expectations`, `require_confirmation`, `require_isolated_ai`,
`reject_regressions` and `allow_evaluation_limitations`. Choose these values as
the owner; there is no automatic personal-quality threshold.

A check uses version 1, `proposal_id`, the exact declared check `name`,
`outcome` (`pass`, `fail`, `unknown`), `actor`, `actor_kind`, `reason` and
preserved `evidence_ids`. Repeat `--check-result` for separate checks. Read the
assessment before making the existing human `workgroup decide` decision with
the same comparison, policy and checks. Do not label AI-authored examples as
human reviewed. See [the contract](../contracts/review-v1.md) for provenance,
disclosure, retention and single-owner limitations.
