-- The deterministic writers and the model's reading are two DIFFERENT claims
-- about one source, and the record's own claim must not be silenced by a
-- reading that got there first.
--
-- The old uniqueness — (deal, criterion, source_type, source_id) — could not
-- tell them apart. A queued reading of a booked meeting can write
-- event_held met=false from its body, and when the meeting is later marked
-- held the deterministic writer's INSERT collides and ON CONFLICT DO NOTHING
-- keeps the model's answer. The record said the meeting happened; the ledger
-- says it did not, and nothing failed.
--
-- Adding extracted_by to the key gives each writer its own row per source, so
-- both claims stand and a reader can see the disagreement. Which one a policy
-- function believes is that function's decision to state, and it can only make
-- it if both rows are there.
--
-- Idempotency is preserved where it mattered: a redelivered event still
-- collides with its OWN writer's row.
-- Bounded: swapping a unique index takes a lock that blocks writers on a
-- table this migration did not create, and an open transaction holding a
-- conflicting lock would otherwise stall every write to it indefinitely.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS deal_stage_evidence_one_per_source_ux;

CREATE UNIQUE INDEX deal_stage_evidence_one_per_source_ux
    ON deal_stage_evidence USING btree (deal_id, criterion_id, source_type, source_id, extracted_by);
