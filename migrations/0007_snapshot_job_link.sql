ALTER TABLE repository_snapshots
    ADD COLUMN job_id uuid UNIQUE REFERENCES jobs(id) ON DELETE SET NULL;
