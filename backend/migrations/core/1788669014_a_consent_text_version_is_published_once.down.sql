SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_event DROP COLUMN IF EXISTS consent_text_version_id;
DROP TRIGGER IF EXISTS consent_text_version_no_edit_after_publish ON consent_text_version;
DROP FUNCTION IF EXISTS consent_text_version_is_immutable();
DROP TABLE IF EXISTS consent_text_version;
