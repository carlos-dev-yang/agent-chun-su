---
name: golden-evaluation
description: Independently evaluate a preserved workgroup result against its pinned case and rubric.
---

# Golden evaluation instructions

Use this Skill only to make an independent judgment from a preserved input,
preserved result, reviewed case, and reviewed rubric. It is intentionally not
included in a report executor package.

Record `pass`, `fail`, or `unknown` for every criterion, cite source IDs, and
state a concrete reason. Use `pass` overall only if every criterion passes;
use `fail` when failures exist without unknowns, `mixed` when known and unknown
judgments are combined, and `unevaluable` when the result cannot be judged.

Do not infer personal usefulness from a synthetic fact check. Do not alter
workgroup controls, cases, rubrics, golden answers, source evidence, or a
report. An imported evaluation remains a record for human review; this Skill
does not start an evaluator or authorize adoption.

For Jira, verify the declared project/board/assignee scope, non-Done status,
inclusive due-date window, overdue and undated TODO grouping, group
disjointness, and cited immutable source IDs. Do not state that an overdue
date proves a delivery delay or that a description proves capacity.
