SET LOCAL lock_timeout = '5s';
-- A foreign-site question becomes the reason it was filed under before.
UPDATE company_domain_disposition
   SET pending_reason = 'unevidenced', updated_at = now()
 WHERE pending_reason = 'foreign_site';
ALTER TABLE company_domain_disposition DROP CONSTRAINT IF EXISTS company_domain_disposition_pending_reason_check;
ALTER TABLE company_domain_disposition ADD CONSTRAINT company_domain_disposition_pending_reason_check CHECK (pending_reason IS NULL OR pending_reason IN ('unevidenced', 'stale_evidence', 'near_duplicate', 'forwarded_identity', 'vendor_verdict', 'reserved_domain')) NOT VALID;
