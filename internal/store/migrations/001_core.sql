CREATE TABLE jobs (
  id TEXT PRIMARY KEY,
  workgroup TEXT NOT NULL,
  input_ref TEXT NOT NULL,
  status TEXT NOT NULL,
  current_attempt TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  not_before INTEGER NOT NULL DEFAULT 0,
  diagnostic TEXT NOT NULL DEFAULT '',
  request_json TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX jobs_queue ON jobs(status, not_before, created_at);
CREATE TABLE attempts (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES jobs(id),
  ordinal INTEGER NOT NULL,
  status TEXT NOT NULL,
  started_at INTEGER NOT NULL,
  ended_at INTEGER NOT NULL DEFAULT 0,
  executor TEXT NOT NULL DEFAULT '',
  diagnostic TEXT NOT NULL DEFAULT '',
  UNIQUE(job_id, ordinal)
);
CREATE TABLE events (
  sequence INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id TEXT NOT NULL REFERENCES jobs(id),
  attempt_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  data_json TEXT NOT NULL
);
CREATE INDEX events_job ON events(job_id, sequence);
CREATE TABLE artifacts (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES jobs(id),
  attempt_id TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  path TEXT NOT NULL UNIQUE,
  digest TEXT NOT NULL,
  bytes INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX artifacts_job ON artifacts(job_id, created_at);
