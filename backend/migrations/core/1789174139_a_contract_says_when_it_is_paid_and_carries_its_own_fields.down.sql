SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
ALTER TABLE custom_field
    ADD CONSTRAINT custom_field_object_check
    CHECK (object = ANY (ARRAY[
        'contact', 'company', 'deal', 'lead',
        'activity', 'project', 'relationship', 'partner'
    ]::text[]));

ALTER TABLE contract DROP CONSTRAINT contract_payment_term_days_nonnegative;
ALTER TABLE contract DROP COLUMN payment_term_days;
