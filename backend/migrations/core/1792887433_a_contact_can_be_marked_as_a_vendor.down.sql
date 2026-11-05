SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    DROP CONSTRAINT IF EXISTS contact_relationship_mark_set_by_pairs,
    DROP CONSTRAINT IF EXISTS contact_relationship_mark_check,
    DROP COLUMN IF EXISTS relationship_mark_set_by,
    DROP COLUMN IF EXISTS relationship_mark;
