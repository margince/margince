-- The newest mail that argued this domain is worth a company.
--
-- A domain triaged from a decade-old thread is answered on evidence nobody has
-- refreshed since: the site it crawls today belongs to whoever owns the domain
-- NOW, and attaching a decade of contacts to them is a wrong answer told
-- confidently. NULL means the row predates this column and is treated as fresh,
-- because the alternative is withholding every company on an empty value.
-- Bounded, because every statement below takes ACCESS EXCLUSIVE on a table the
-- capture path writes to on every message: an open transaction holding a
-- conflicting lock would otherwise stall all of that for as long as this
-- migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    ADD COLUMN IF NOT EXISTS last_evidence_at timestamptz;

-- 'stale_evidence' joins 'unevidenced' as a reason a question is left open.
-- The two are different states and the queue renders them differently: one has
-- no evidence at all, the other has evidence too old to mint from.
ALTER TABLE company_domain_disposition
    DROP CONSTRAINT IF EXISTS company_domain_disposition_pending_reason_check;
ALTER TABLE company_domain_disposition
    ADD CONSTRAINT company_domain_disposition_pending_reason_check
    CHECK (pending_reason IS NULL OR pending_reason IN ('unevidenced', 'stale_evidence'));
