-- A record's history includes the LINKS made and removed on it, which means
-- looking up relationship rows by any endpoint the record could occupy.
--
-- Every existing index on those columns is PARTIAL on `archived_at IS NULL`
-- (idx_rel_traverse_*, idx_rel_person_orgs, idx_rel_partner_counterparty), and
-- two are kind-scoped as well. A history read cannot use them: an UNLINK is
-- exactly the event that archives the row, and a history that hides the unlink
-- is the one thing this read exists to show. Without these, the lookup
-- seq-scans relationship on every page of every record's history.
--
-- Five indexes rather than one composite: the predicate is "this record is at
-- ANY end", which is five equality lookups unioned, not one ordered prefix.
-- The write cost lands on linking and unlinking records, which no bulk path
-- drives.
--
-- Not CONCURRENTLY: a migration runs in one transaction and CONCURRENTLY forbids
-- that (see 1787650813 and 1787320004 on the same point), so each build holds a
-- write-blocking SHARE lock for its duration. Stated rather than left to be
-- found: these five are NON-partial, so unlike the scrub-boundary index each
-- scans the whole of `relationship` rather than a predicate's slice of it. That
-- is the bound worth knowing, and it is what makes it acceptable on THIS table:
-- one row per link between two records, where audit_log holds one per mutation
-- of every record there has ever been.
--
-- SET LOCAL lock_timeout bounds how long each CREATE INDEX waits to ACQUIRE its
-- SHARE lock on `relationship`, not how long it holds it. The table is live and
-- this file did not create it, so an unbounded wait would sit behind an open
-- transaction with every writer queued behind THIS statement — a deploy that
-- stalls linking and unlinking instead of failing fast and being retried.
SET LOCAL lock_timeout = '3s';

CREATE INDEX IF NOT EXISTS idx_rel_history_person
    ON relationship USING btree (person_id);
CREATE INDEX IF NOT EXISTS idx_rel_history_organization
    ON relationship USING btree (organization_id);
CREATE INDEX IF NOT EXISTS idx_rel_history_counterparty
    ON relationship USING btree (counterparty_org_id);
CREATE INDEX IF NOT EXISTS idx_rel_history_deal
    ON relationship USING btree (deal_id);
CREATE INDEX IF NOT EXISTS idx_rel_history_project
    ON relationship USING btree (project_id);
