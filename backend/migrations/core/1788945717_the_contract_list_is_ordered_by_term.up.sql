-- The account's contract list answers "which agreement is current", so it is
-- ordered by TERM, not by when the row was written.
--
-- The index followed the query rather than the promise: it was moved to
-- created_at when the query was, so the page was internally consistent and
-- wrong. An older agreement imported today sorted ahead of a newer one created
-- last week, and the account page read as though the wrong agreement was
-- current.
--
-- NULLS LAST is spelled out because Postgres defaults a DESC index to NULLS
-- FIRST, and the query orders `starts_on DESC NULLS LAST` — an index whose null
-- placement disagrees with the ORDER BY cannot serve the sort, so the page
-- would silently fall back to a sort node over the whole account.
--
-- created_at stays in the key, after starts_on: it is the tiebreak among terms
-- that begin on the same day, and among imported agreements that carry no start
-- date at all — where it is the only ordering left.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS contract_account_ix;

CREATE INDEX contract_account_ix ON contract
    USING btree (organization_id, starts_on DESC NULLS LAST, created_at DESC, id DESC)
    WHERE (archived_at IS NULL);
