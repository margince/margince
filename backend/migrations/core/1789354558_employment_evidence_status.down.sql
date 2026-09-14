SET LOCAL lock_timeout = '3s';

-- Refuse rollback while explicit historical/unknown undated records would change meaning.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM relationship WHERE employment_status IN ('former','unknown') AND ended_at IS NULL AND archived_at IS NULL) THEN
  RAISE EXCEPTION 'Resolve undated employment evidence before rolling back employment status';
 END IF;
END $$;
DROP INDEX provider_employment_pending;
ALTER TABLE contact_provider_claim DROP COLUMN employment_processed_at;
DROP TABLE provider_employment_resolution;

DROP INDEX uq_rel_employment;
ALTER TABLE relationship DROP CONSTRAINT rel_employment_primary_status;
ALTER TABLE relationship DROP CONSTRAINT rel_employment_status_kind;
ALTER TABLE relationship DROP COLUMN ended_precision;
ALTER TABLE relationship DROP COLUMN started_precision;
ALTER TABLE relationship DROP COLUMN employment_status;
CREATE UNIQUE INDEX uq_rel_employment ON relationship (contact_id, company_id)
 WHERE kind = 'employment' AND ended_at IS NULL AND archived_at IS NULL;
