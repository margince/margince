SET LOCAL lock_timeout = '5s';

-- Two rules the lead row was leaving to whoever happened to write it.
--
-- `uq_lead_ws_id` was UNIQUE (workspace_id, id) until the tenant column was
-- dropped, and collapsed to UNIQUE (id) — which `lead_pkey` already enforces on
-- the same column. A second index on the primary key is written on every insert
-- and chosen by no plan. Nothing names it in `ON CONFLICT ON CONSTRAINT`.
ALTER TABLE lead DROP CONSTRAINT IF EXISTS uq_lead_ws_id;

-- The score columns are looser than the table that records what they WERE:
-- lead_score_history carries both as smallint CHECK 0..100, so the history
-- could not faithfully record a value its own parent allowed. The range CHECKs
-- landed already (lead_score_range, lead_score_computed_range); the width did
-- not, and the two halves of one rule are worth spelling the same way.
--
-- Safe as a rewrite because the CHECKs are already in force: no live value can
-- be outside 0..100, so no row can fail the cast. The match is with
-- company.relevance and partner.partner_fit_score, both smallint 0..100.
ALTER TABLE lead
    ALTER COLUMN score TYPE smallint,
    ALTER COLUMN score_computed TYPE smallint;
