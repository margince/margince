-- Puts the three Surfe-only CHECK constraints back, and the two defaults with
-- them, exactly as the 0001 baseline declared them.
--
-- This can FAIL, and failing is correct. If a second provider was connected
-- while the up migration was in force, rows naming it exist and the constraint
-- refuses to validate — which is the honest answer: going back to a schema that
-- admits one vendor while holding another vendor's purchases would either lose
-- data or leave a constraint everybody has to pretend is true.
--
-- An operator who genuinely needs this must first disconnect the second
-- provider and erase what it sold, through DeleteProviderData, which scrubs the
-- claims and the runs as one write with its audit trail. That is a product
-- operation with a receipt, not something a migration should do silently to
-- somebody's paid-for data.

-- Bounded for the reason the up migration states: these ALTERs block writers
-- on tables they did not create, and an unbounded wait stalls every write for
-- as long as a conflicting transaction stays open.
SET LOCAL lock_timeout = '3s';

ALTER TABLE person_provider_claim
    ALTER COLUMN captured_by SET DEFAULT 'connector:surfe'::text;

ALTER TABLE person_provider_claim
    ALTER COLUMN source SET DEFAULT 'surfe'::text;

ALTER TABLE person_provider_claim
    ADD CONSTRAINT person_provider_claim_provider_check CHECK ((provider = 'surfe'::text));

ALTER TABLE provider_run
    ADD CONSTRAINT provider_run_provider_check CHECK ((provider = 'surfe'::text));

ALTER TABLE provider_connection
    ADD CONSTRAINT provider_connection_provider_check CHECK ((provider = 'surfe'::text));
