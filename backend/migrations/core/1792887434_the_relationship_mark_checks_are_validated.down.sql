SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    DROP CONSTRAINT contact_relationship_mark_check,
    ADD CONSTRAINT contact_relationship_mark_check CHECK (relationship_mark = ANY (ARRAY['vendor_marketing']))
        NOT VALID,
    DROP CONSTRAINT contact_relationship_mark_set_by_pairs,
    ADD CONSTRAINT contact_relationship_mark_set_by_pairs CHECK (
        (relationship_mark IS NULL) = (relationship_mark_set_by IS NULL)) NOT VALID;
