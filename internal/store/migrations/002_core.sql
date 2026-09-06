CREATE TABLE records (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  subject_id TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL UNIQUE,
  digest TEXT NOT NULL,
  bytes INTEGER NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX records_kind_created ON records(kind, created_at);
CREATE INDEX records_subject ON records(subject_id, created_at);
