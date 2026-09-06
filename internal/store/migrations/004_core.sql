CREATE TABLE local_settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO local_settings(key,value) VALUES('queue_paused','false');
CREATE TABLE schedules (
  id TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL,
  name TEXT NOT NULL,
  interval_ms INTEGER NOT NULL CHECK(interval_ms > 0),
  timezone TEXT NOT NULL,
  next_run_at INTEGER NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 0,
  last_tick_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE schedule_ticks (
  id TEXT PRIMARY KEY,
  schedule_id TEXT NOT NULL REFERENCES schedules(id),
  scheduled_for INTEGER NOT NULL,
  triggered_at INTEGER NOT NULL,
  missed_intervals INTEGER NOT NULL,
  acquisition_id TEXT NOT NULL,
  continue_id TEXT NOT NULL DEFAULT '',
  job_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  not_before INTEGER NOT NULL DEFAULT 0,
  diagnostic TEXT NOT NULL DEFAULT '',
  collection_attempts INTEGER NOT NULL DEFAULT 0,
  UNIQUE(schedule_id,scheduled_for)
);
CREATE INDEX schedule_tick_state ON schedule_ticks(status,not_before,triggered_at);
ALTER TABLE artifacts ADD COLUMN content_state TEXT NOT NULL DEFAULT 'available';
CREATE TABLE retention_operations (
  id TEXT PRIMARY KEY,
  job_id TEXT NOT NULL REFERENCES jobs(id),
  status TEXT NOT NULL,
  plan_path TEXT NOT NULL,
  plan_digest TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
