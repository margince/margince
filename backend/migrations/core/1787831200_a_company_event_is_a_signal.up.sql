-- Four kinds for what a company says about itself in public.
--
-- A newsroom item is a signal about an ACCOUNT — a funding round, a new CFO, a
-- second office, a product — and the signal table is already the company-level
-- substrate with the fingerprint dedupe, the triage lifecycle and the surface
-- that lists them. What it lacked was a vocabulary for the four things a
-- company's own press page actually announces.
--
-- Deliberately NOT folded onto `new_opportunity`, which is the closest existing
-- kind and the wrong one: that kind carries workflow meaning for the warm-room
-- and resolver paths, and a quarter's worth of press releases arriving under it
-- would put newsroom noise into a surface built for sales intent.
--
-- `leadership_change` names a person in its summary and is still a COMPANY
-- event: the resolver never creates a person from a signal, and a new CFO is a
-- fact about the account until somebody decides otherwise.
--
-- ADD ... NOT VALID then VALIDATE, then swap: the runner wraps each migration
-- in one transaction, so a failure at any statement rolls the whole file back
-- and `signal` can never be left with no kind constraint. The two-step also
-- shortens the ACCESS EXCLUSIVE hold — NOT VALID takes it without scanning, and
-- VALIDATE drops to SHARE UPDATE EXCLUSIVE for the pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal ADD CONSTRAINT signal_kind_check_v2
    CHECK (kind IN ('stalled_deal', 'champion_left', 'reengagement', 'buying_intent',
                    'risk', 'other', 'contract_ended', 'new_opportunity',
                    'commitment_made', 'ghosted_thread', 'project_gone_quiet',
                    'funding', 'leadership_change', 'expansion', 'product_launch'))
    NOT VALID;

ALTER TABLE signal VALIDATE CONSTRAINT signal_kind_check_v2;

ALTER TABLE signal DROP CONSTRAINT signal_kind_check;

ALTER TABLE signal RENAME CONSTRAINT signal_kind_check_v2 TO signal_kind_check;
