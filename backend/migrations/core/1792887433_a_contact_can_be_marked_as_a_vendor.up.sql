-- A contact can carry a mark that says what the workspace's relationship to it is.
--
-- Today the only mark is vendor_marketing: a named human whose mail only pitches
-- something to this business and whom nobody here ever answered. The contact
-- exists, so the mail stays attached to somebody, but the readers that turn
-- contacts into relationship signals (lists, worklist, briefs) skip it.
--
-- relationship_mark_set_by records who set it in the grammar of captured_by
-- (human:<uuid> or agent:<id>), because only a mark set by the verdict may be
-- lifted automatically when the workspace later corresponds with the address.
-- NULL means "not marked"; no existing row is changed.
--
-- Both constraints are added NOT VALID; the next migration validates them
-- under the lighter lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    ADD COLUMN IF NOT EXISTS relationship_mark text,
    ADD COLUMN IF NOT EXISTS relationship_mark_set_by text,
    ADD CONSTRAINT contact_relationship_mark_check CHECK (relationship_mark = ANY (ARRAY['vendor_marketing']))
        NOT VALID,
    ADD CONSTRAINT contact_relationship_mark_set_by_pairs CHECK (
        (relationship_mark IS NULL) = (relationship_mark_set_by IS NULL)) NOT VALID;

COMMENT ON COLUMN contact.relationship_mark IS
    'What the workspace''s relationship to the contact is when it is not an ordinary one: vendor_marketing means a named sender who only pitches and was never answered. NULL is not marked.';
