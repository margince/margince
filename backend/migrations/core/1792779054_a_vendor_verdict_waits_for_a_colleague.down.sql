SET LOCAL lock_timeout = '5s';
UPDATE company_domain_disposition
   SET status = 'provider', source = 'site_read', pending_reason = NULL, next_attempt_at = NULL, updated_at = now()
 WHERE pending_reason = 'vendor_verdict';
ALTER TABLE company_domain_disposition DROP CONSTRAINT company_domain_disposition_pending_reason_check;
ALTER TABLE company_domain_disposition ADD CONSTRAINT company_domain_disposition_pending_reason_check CHECK (pending_reason IS NULL OR pending_reason IN ('unevidenced', 'stale_evidence', 'near_duplicate', 'forwarded_identity', 'reserved_domain')) NOT VALID;
