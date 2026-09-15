-- A row withheld for stale evidence has no older spelling to return to, so the
-- reason is cleared rather than translated: the domain goes back to being an
-- ordinary open question, which is what it was before this column existed.
SET LOCAL lock_timeout = '3s';

-- The CURSOR is rearmed with the reason, not just cleared beside it. A withheld
-- row has next_attempt_at NULL, which is what keeps the sweep from re-crawling
-- evidence that cannot improve — and after a rollback nothing is left that
-- understands 'stale_evidence' well enough to rearm it, so the domain would sit
-- outside ListDueDomains for ever.
UPDATE company_domain_disposition
   SET pending_reason = NULL, next_attempt_at = now(), attempts = 0
 WHERE pending_reason = 'stale_evidence';

ALTER TABLE company_domain_disposition
    DROP CONSTRAINT IF EXISTS company_domain_disposition_pending_reason_check;
ALTER TABLE company_domain_disposition
    ADD CONSTRAINT company_domain_disposition_pending_reason_check
    CHECK (pending_reason IS NULL OR pending_reason = 'unevidenced');

ALTER TABLE company_domain_disposition
    DROP COLUMN IF EXISTS last_evidence_at;
