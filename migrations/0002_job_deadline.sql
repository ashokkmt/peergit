ALTER TABLE jobs ADD COLUMN deadline_at timestamptz;
CREATE INDEX jobs_deadline_idx ON jobs (deadline_at, id)
    WHERE deadline_at IS NOT NULL AND state IN ('ready', 'running');
