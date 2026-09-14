-- A suppression can bind ONE marketing purpose, not only every marketing message.
--
-- Until now a row bound by KIND: a marketing_objection stopped all marketing. That
-- is the only stop a lead could hold, because a lead has no per-purpose consent row
-- and this table had no purpose column — so a lead's newsletter-specific unsubscribe
-- had nowhere to land and silently recorded nothing.
--
-- NULL is the whole existing table and stays "all marketing", so this is additive and
-- needs no backfill. A non-null purpose_id narrows the row to that consent_purpose, and
-- the send engine binds it only when the message resolves to the same purpose.
SET LOCAL lock_timeout = '3s';   -- read on every send; do not queue behind a long txn
ALTER TABLE communication_suppression ADD COLUMN purpose_id uuid;

-- The FK goes on NOT VALID here and is validated in a LATER migration. One file
-- is one transaction (dbmigrate.Up), so validating in this file would hold the
-- ADD's lock through the whole validating scan of a table read on every send;
-- the split across two transactions is the only form that lets writers through
-- between the add and the scan. RESTRICT to match the five sibling FKs to
-- consent_purpose — a purpose still named by a live stop may not be deleted.
ALTER TABLE communication_suppression
    ADD CONSTRAINT communication_suppression_purpose_id_fkey
    FOREIGN KEY (purpose_id) REFERENCES consent_purpose(id) ON DELETE RESTRICT NOT VALID;
