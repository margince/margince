-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded, because ALTER TABLE takes ACCESS EXCLUSIVE on a table this
-- migration did not create: a writer holding a row lock would otherwise queue
-- every reader behind this statement for as long as it cared to wait. Two
-- seconds is the tree's own number for an additive column.
SET LOCAL lock_timeout = '2s';
-- A stripped request records what was removed.
--
-- The secret stripper already answers this: model.StripReport carries a count
-- and the kinds, and its own doc says what it is for -- "StripReport says what
-- was removed, FOR THE AUDIT TRAIL". Every real adapter then discarded it with
-- `_`. A control that exists for an audit and leaves no witness cannot answer
-- the one question an audit asks: what did we remove from that request.
--
-- On ai_call, which is the ATTEMPT row, because that is what the report
-- describes -- one marshalled body, stripped once, per attempt. Putting it on
-- the logical call would average away the case worth seeing: a retry whose body
-- differed from the first.
--
-- NO CHECK on the kinds. The rule set is build-side (ai/stripper.go) and a new
-- rule must not need a migration to be recordable; a constraint here would be a
-- third spelling of that list, and the one furthest from the code that decides
-- it. What the array records is what the stripper reported, whatever it reports.
--
-- Backfill is deliberately absent. Rows written before this migration were
-- stripped by the same pass and nobody kept the answer, so 0 and {} are honest
-- for them in exactly the way a fabricated number would not be.
ALTER TABLE ai_call
    ADD COLUMN secrets_removed bigint DEFAULT 0 NOT NULL,
    ADD COLUMN secret_kinds text[] DEFAULT '{}'::text[] NOT NULL,
    ADD CONSTRAINT ai_call_secrets_removed_check CHECK (secrets_removed >= 0);
