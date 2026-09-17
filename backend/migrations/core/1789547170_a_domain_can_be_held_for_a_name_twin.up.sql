-- A domain triage question can now be left open because the name the site
-- stated already belongs to a company here.
--
-- 'near_duplicate' joins the two reasons a question stays pending. It is not
-- the same state as either: 'unevidenced' has nothing to mint from and
-- 'stale_evidence' has only evidence too old to mint from, while this one has
-- perfectly good evidence and a rival — creating would put one company in the
-- workspace twice, which is the defect, and merging on a near-match is a
-- human's call.

-- The table is read by the domain sweep and both open-question lists, so a
-- conflicting lock would stall all of them for as long as this migration is
-- willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    DROP CONSTRAINT IF EXISTS company_domain_disposition_pending_reason_check;
ALTER TABLE company_domain_disposition
    ADD CONSTRAINT company_domain_disposition_pending_reason_check
    CHECK (pending_reason IS NULL OR pending_reason IN ('unevidenced', 'stale_evidence', 'near_duplicate'));
