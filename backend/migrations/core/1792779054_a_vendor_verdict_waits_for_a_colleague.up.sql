SET LOCAL lock_timeout = '5s';
ALTER TABLE company_domain_disposition DROP CONSTRAINT company_domain_disposition_pending_reason_check;
ALTER TABLE company_domain_disposition ADD CONSTRAINT company_domain_disposition_pending_reason_check CHECK (pending_reason IS NULL OR pending_reason IN ('unevidenced', 'stale_evidence', 'near_duplicate', 'forwarded_identity', 'vendor_verdict', 'reserved_domain')) NOT VALID;
-- A vendor verdict the site read settled with nobody asked becomes an open
-- question. A human answer or an admission stands.
UPDATE company_domain_disposition
   SET status = 'pending', source = NULL, pending_reason = 'vendor_verdict', next_attempt_at = NULL, updated_at = now()
 WHERE status = 'provider' AND source = 'site_read' AND admission IS NULL;
