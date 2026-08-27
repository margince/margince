-- The three tables that name a licensed data provider stop naming only Surfe.
--
-- Every index and unique constraint on these tables was already keyed by
-- provider: provider_connection holds one row PER provider, provider_run's
-- live-run index is (person_id, provider, input_fingerprint), and
-- person_provider_claim's is (person_id, provider, retrieved_at). The schema
-- has been ready for a second vendor since the baseline. What pinned it was
-- three CHECK constraints, each spelling the one adapter that existed when
-- they were written.
--
-- Dropping them is what lets the person record show a section per connected
-- provider, each saying who was paid for which value. Until now the page could
-- only say "the connected data provider", because only one could exist.
--
-- The value is NOT re-constrained to a wider list here, deliberately. Which
-- providers exist is a deployment fact, not a schema one: the registry refuses
-- a name no adapter is compiled for (integrations.ErrUnknownProvider, a 404),
-- and the run path resolves every name through it before a row is written. A
-- CHECK listing vendors would be a second copy of that list, and the copy is
-- what goes stale — this is the same reasoning PersonReachability.Provider
-- already states in the contract for its own provider field.
--
-- Nothing is rewritten. Existing rows all say 'surfe' and still satisfy an
-- absent constraint, so this is additive in effect: no backfill, no lock beyond
-- the catalog update, and an older binary keeps working because it only ever
-- writes the value the constraint used to demand.

ALTER TABLE provider_connection
    DROP CONSTRAINT provider_connection_provider_check;

ALTER TABLE provider_run
    DROP CONSTRAINT provider_run_provider_check;

ALTER TABLE person_provider_claim
    DROP CONSTRAINT person_provider_claim_provider_check;

-- The two DEFAULTs beside them named Surfe too. They are dead — the writer in
-- modules/people passes both columns explicitly, deriving captured_by from the
-- run's own provider ('connector:' || provider) — but a default that names one
-- vendor on a table that now holds several is a lie waiting for the first
-- INSERT that omits the column.
ALTER TABLE person_provider_claim
    ALTER COLUMN source DROP DEFAULT;

ALTER TABLE person_provider_claim
    ALTER COLUMN captured_by DROP DEFAULT;
