SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship DROP CONSTRAINT rel_dates;
ALTER TABLE relationship ADD CONSTRAINT rel_dates CHECK (ended_at IS NULL OR started_at IS NULL OR ended_at >= started_at);
