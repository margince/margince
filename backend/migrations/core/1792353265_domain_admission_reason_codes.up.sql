SET LOCAL lock_timeout = '5s';

ALTER TABLE company_domain_disposition
    ADD COLUMN admission_reason_code text,
    ADD COLUMN admission_sender_kind text;
