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
ALTER TABLE communication_suppression
    ADD COLUMN purpose_id uuid REFERENCES consent_purpose(id);
