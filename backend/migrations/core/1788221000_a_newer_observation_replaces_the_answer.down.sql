SET LOCAL lock_timeout = '3s';

ALTER TABLE person_profile_field DROP CONSTRAINT person_profile_field_field_check;

DELETE FROM person_profile_field WHERE field IN ('address', 'website');

ALTER TABLE person_profile_field
    ADD CONSTRAINT person_profile_field_field_check
    CHECK (field IN ('title', 'phone', 'role', 'linkedin', 'org_name'));

ALTER TABLE person_phone
    DROP COLUMN superseded_phone_id,
    DROP COLUMN observed_at;

ALTER TABLE person_profile_field
    DROP COLUMN superseded_observed_at,
    DROP COLUMN superseded_captured_by,
    DROP COLUMN superseded_value,
    DROP COLUMN observed_at;
