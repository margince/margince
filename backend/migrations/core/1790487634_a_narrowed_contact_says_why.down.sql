SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    DROP CONSTRAINT IF EXISTS contact_narrowing_reason_only_when_owner,
    DROP CONSTRAINT IF EXISTS contact_narrowing_reason_check,
    DROP COLUMN IF EXISTS narrowing_reason;
