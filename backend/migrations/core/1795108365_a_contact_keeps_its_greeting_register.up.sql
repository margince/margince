-- A contact can record how a message to them opens: formal (surname) or familiar (first name).
--
-- The rep sets it once and every draft to the contact uses it before reading the thread.
-- NULL means nobody set it, and the draft falls back to the register the correspondence shows.
-- No existing row changes. The check is added NOT VALID; the next migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    ADD COLUMN IF NOT EXISTS greeting_register text,
    ADD CONSTRAINT contact_greeting_register_check CHECK (greeting_register = ANY (ARRAY['formal', 'familiar']))
        NOT VALID;

COMMENT ON COLUMN contact.greeting_register IS
    'How a message to the contact opens: formal (surname) or familiar (first name). Set by a rep; NULL means the draft reads the register from the correspondence.';
