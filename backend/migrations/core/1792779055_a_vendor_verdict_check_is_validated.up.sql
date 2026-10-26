SET LOCAL lock_timeout = '3s';
ALTER TABLE company_domain_disposition VALIDATE CONSTRAINT company_domain_disposition_pending_reason_check;
