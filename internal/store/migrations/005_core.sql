CREATE TABLE workflow_jobs (
  job_id TEXT PRIMARY KEY REFERENCES jobs(id),
  origin_conversation_id TEXT NOT NULL,
  origin_message_id TEXT NOT NULL,
  request_revision INTEGER NOT NULL CHECK(request_revision >= 0),
  original_request_json TEXT NOT NULL,
  completion_criteria_json TEXT NOT NULL,
  source_scope_json TEXT NOT NULL,
  budget_json TEXT NOT NULL,
  destination_json TEXT NOT NULL,
  workflow_version TEXT NOT NULL,
  paused INTEGER NOT NULL DEFAULT 0 CHECK(paused IN (0,1)),
  manual_checkpoint INTEGER NOT NULL DEFAULT 0 CHECK(manual_checkpoint IN (0,1)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX workflow_jobs_conversation ON workflow_jobs(origin_conversation_id,created_at);

CREATE TABLE workflow_steps (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES workflow_jobs(job_id),
  step_key TEXT NOT NULL,
  stage TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  dependencies_json TEXT NOT NULL DEFAULT '[]',
  pinned_inputs_json TEXT NOT NULL DEFAULT '[]',
  expected_output_schema TEXT NOT NULL,
  executor_model TEXT NOT NULL,
  executor_effort TEXT NOT NULL,
  executor_identity TEXT NOT NULL,
  instruction_version TEXT NOT NULL,
  gate_policy TEXT NOT NULL DEFAULT 'auto',
  state TEXT NOT NULL DEFAULT 'pending',
  current_attempt_id TEXT NOT NULL DEFAULT '',
  output_artifact_id TEXT NOT NULL DEFAULT '',
  not_before INTEGER NOT NULL DEFAULT 0,
  diagnostic TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE(job_id,step_key),
  UNIQUE(job_id,ordinal)
);
CREATE INDEX workflow_steps_job_state ON workflow_steps(job_id,state,ordinal);

CREATE TABLE step_attempts (
  id TEXT PRIMARY KEY,
  step_id TEXT NOT NULL REFERENCES workflow_steps(id),
  ordinal INTEGER NOT NULL,
  status TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  ended_at INTEGER NOT NULL DEFAULT 0,
  inputs_json TEXT NOT NULL DEFAULT '[]',
  output_artifact_id TEXT NOT NULL DEFAULT '',
  executor_model TEXT NOT NULL,
  executor_effort TEXT NOT NULL,
  executor_identity TEXT NOT NULL,
  instruction_version TEXT NOT NULL,
  diagnostic TEXT NOT NULL DEFAULT '',
  UNIQUE(step_id,ordinal)
);
CREATE INDEX step_attempts_step ON step_attempts(step_id,ordinal);

CREATE TABLE result_events (
  id TEXT PRIMARY KEY,
  event_key TEXT NOT NULL UNIQUE,
  job_id TEXT NOT NULL REFERENCES workflow_jobs(job_id),
  step_id TEXT NOT NULL REFERENCES workflow_steps(id),
  conversation_id TEXT NOT NULL,
  request_revision INTEGER NOT NULL,
  artifact_id TEXT NOT NULL REFERENCES artifacts(id),
  artifact_digest TEXT NOT NULL,
  summary TEXT NOT NULL,
  source_refs_json TEXT NOT NULL DEFAULT '[]',
  gaps_json TEXT NOT NULL DEFAULT '[]',
  status TEXT NOT NULL,
  required_decision_json TEXT NOT NULL DEFAULT 'null',
  delivery_status TEXT NOT NULL DEFAULT 'available',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX result_events_handoff ON result_events(conversation_id,delivery_status,created_at);
