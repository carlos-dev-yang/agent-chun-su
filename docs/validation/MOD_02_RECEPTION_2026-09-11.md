# MOD-02 — independent reception and module admission

The reception engine now lives outside CLI. Terminal and paired Telegram
transports call the same history/action loop and host dispatcher. Secret-input
and local display callbacks stay in the local UI. Workgroup input validation,
source-index construction and exact tool identity come from compiled module
definitions. The existing mail/Jira lifecycle, result contracts and live-source
checks remain in use.

Actual macOS/arm64 walkthrough with Codex CLI 0.153.4 / gpt-5.5:

- Natural language requested reprocessing saved synthetic job
  `WIGQRJO4PKWBGQQQYOYAQNVKL2`. Reception dispatched `delegate_job` and returned
  new task `2MBVRQE4LKHKPMUYKM4TUJHR3R` as queued, explicitly not completed.
- A separate runner invocation generated a report in attempt
  `MZURIGDBU2DN5RKDWYVJ4S22YY`, with its own pinned mail Skill, source index,
  exact-source gateway evidence and host validation. Its ambiguous source
  produced `waiting_input`. The task package contains no evaluator/golden files.
- A request to add shell permission and modify active controls/SQLite was
  refused with no host action. Reception receipts contain no native tool use.
- Reception shutdown stopped its setup-only child normally. Saved reprocessing
  does not advance collection coverage, including later publication recovery.

Existing feedback/runner tests passed after the module extraction and reception
changes; affected-package vet and build passed. No new test code was created.
Telegram network delivery and real account authentication were not repeated.
Its transport now calls the common engine; existing pairing, durable receipt,
cancel/reset and at-most-once update handling remain outside that engine.

The module registry is compiled, not a dynamic arbitrary-code loader. Domain
result validation and live-source proof still have explicit adapters. Reception
currently delegates previously admitted saved input; new account ingestion and
new development permissions require their own bounded admission contracts.
