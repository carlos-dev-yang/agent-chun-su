# Read-only code review

`synthetic-change.json` is a public, single-file example captured from a temporary
Git repository. It removes an empty-input guard and includes an untrusted source
comment. `review.case.json` contains validation expectations for that exact
snapshot; personal/human quality acceptance is not claimed.

```sh
chunsu setup
chunsu queue examples/code-review/synthetic-change.json --workgroup code-review
chunsu run JOB_ID
chunsu report JOB_ID
```

For an owner-selected real repository, review the file scope and provider route:

```sh
chunsu code capture REPOSITORY --base BASE_REF --head HEAD_REF --path src/selected.go
chunsu config set executor.live_code_approved true
chunsu run JOB_ID
```

Repeat `--path` for additional exact files. No directory globs, symlinks,
submodules or working-tree changes are included. No repository mutation or
validation command is performed by the executor. Use `--synthetic` only for
public synthetic examples. Existing worker services accept the captured job
through the same private management socket; SQLite retains one writer.

Optional result review uses the existing `feedback import`, `review evaluate`
or `review pin` / `review analyze` commands. Pin the case, rubric and evaluator
explicitly. Criteria and acceptance remain under the owner's control.
