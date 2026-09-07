-- Bounded: swapping a unique index takes a lock that blocks writers on a
-- table this migration did not create, and an open transaction holding a
-- conflicting lock would otherwise stall every write to it indefinitely.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS deal_stage_evidence_one_per_source_ux;

CREATE UNIQUE INDEX deal_stage_evidence_one_per_source_ux
    ON deal_stage_evidence USING btree (deal_id, criterion_id, source_type, source_id);
