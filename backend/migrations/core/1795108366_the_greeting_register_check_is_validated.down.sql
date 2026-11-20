SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    DROP CONSTRAINT contact_greeting_register_check,
    ADD CONSTRAINT contact_greeting_register_check CHECK (greeting_register = ANY (ARRAY['formal', 'familiar']))
        NOT VALID;
