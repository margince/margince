-- Every organization with live domains names one of them primary.
--
-- Three readers key on is_primary, and the first of them is why this backfill
-- exists rather than only the code change beside it: the auto-enrich sweep
-- INNER JOINs organization_domain on is_primary, so a company whose only domain
-- was written with the column false is not a candidate and never becomes one.
-- Nothing retries, nothing errors, and no state records the omission — the
-- company simply stays empty. The company read derives website_url from the
-- same column, and provider enrichment reads it for the employer domain.
--
-- The rows this repairs were written through an API that admits the state: the
-- contract defaults is_primary to false and requires only domain, so a caller
-- constructing the minimal valid body lands here. The code now elects a primary
-- at the store for every new record; these are the ones already written.
--
-- The oldest live domain is elected, which is the same tie-break the electing
-- code applies at the door — the first domain the caller offered — carried over
-- to rows whose caller is long gone. id breaks a created_at tie so the choice is
-- deterministic rather than dependent on scan order.
--
-- uq_org_domain_primary is a partial unique index on organization_id where
-- is_primary and archived_at IS NULL. The NOT EXISTS arm is what keeps this
-- inside it: only organizations with NO live primary are touched, and exactly
-- one row is elected per organization. Re-running promotes nothing, because the
-- organizations it would select no longer qualify.
UPDATE organization_domain d
   SET is_primary = true
 WHERE d.archived_at IS NULL
   AND NOT EXISTS (
       SELECT 1 FROM organization_domain held
        WHERE held.organization_id = d.organization_id
          AND held.is_primary
          AND held.archived_at IS NULL)
   AND d.id = (
       SELECT oldest.id FROM organization_domain oldest
        WHERE oldest.organization_id = d.organization_id
          AND oldest.archived_at IS NULL
        ORDER BY oldest.created_at, oldest.id
        LIMIT 1);
