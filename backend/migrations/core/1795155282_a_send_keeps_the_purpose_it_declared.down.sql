SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP COLUMN IF EXISTS consent_purpose,
    DROP COLUMN IF EXISTS consent_class;
