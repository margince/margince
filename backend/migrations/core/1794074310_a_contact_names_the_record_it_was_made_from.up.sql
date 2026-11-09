SET LOCAL lock_timeout = '3s';
-- An agent reads a note and creates a contact from it. Until now the contact
-- kept who made it and for whom, and not what it was made from, so a reader
-- could not check where the name or title came from.
--
-- An activity, because every source a create reads is one: a note, a mail or a
-- quick capture. A polymorphic pair would carry no foreign key, and widening
-- to another record type later only adds a column.
--
-- SET NULL rather than CASCADE: a contact outlives the note it was read from,
-- and the erasure of that note must not take the contact with it.
ALTER TABLE contact
    ADD COLUMN created_from_activity_id uuid
        REFERENCES activity(id) ON DELETE SET NULL;

-- Partial, because the column is null on every contact a human typed.
CREATE INDEX idx_contact_created_from
    ON contact (created_from_activity_id)
    WHERE created_from_activity_id IS NOT NULL;
