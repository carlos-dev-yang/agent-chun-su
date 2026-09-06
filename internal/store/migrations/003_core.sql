CREATE TABLE acquisitions (
  id TEXT PRIMARY KEY,
  connection_id TEXT NOT NULL,
  parent_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  as_of TEXT NOT NULL,
  not_before INTEGER NOT NULL DEFAULT 0,
  path TEXT NOT NULL,
  digest TEXT NOT NULL,
  bytes INTEGER NOT NULL,
  job_id TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX acquisition_connection_state ON acquisitions(connection_id,status,created_at);
CREATE UNIQUE INDEX acquisition_continuation_parent ON acquisitions(parent_id) WHERE parent_id <> '';
CREATE TABLE source_coverage (
  connection_id TEXT NOT NULL,
  source_id TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  job_id TEXT NOT NULL REFERENCES jobs(id),
  reported_at INTEGER NOT NULL,
  PRIMARY KEY(connection_id,source_id)
);
CREATE UNIQUE INDEX job_acquisition_identity ON jobs(json_extract(request_json,'$.acquisition_id'))
  WHERE json_extract(request_json,'$.acquisition_id') IS NOT NULL;
