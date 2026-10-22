SET LOCAL lock_timeout = '5s';

ALTER TABLE company_domain_disposition
    ADD COLUMN admission_reason_coded_at timestamptz;
