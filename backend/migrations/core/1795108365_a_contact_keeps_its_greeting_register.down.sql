SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    DROP CONSTRAINT IF EXISTS contact_greeting_register_check,
    DROP COLUMN IF EXISTS greeting_register;
