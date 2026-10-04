ALTER TABLE repository_snapshots
    ADD COLUMN job_id uuid UNIQUE REFERENCES jobs(id) ON DELETE SET NULL;

CREATE INDEX repository_snapshots_job_idx ON repository_snapshots(job_id) WHERE job_id IS NOT NULL;
