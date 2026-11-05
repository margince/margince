SET LOCAL lock_timeout = '5s';

ALTER TABLE company_domain_disposition
    DROP COLUMN admission_sender_kind,
    DROP COLUMN admission_reason_code;
