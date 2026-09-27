-- Back to present and unvalidated, which is how this migration found it:
-- Postgres has no ALTER that un-validates, so it is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_profile_field
    DROP CONSTRAINT contact_profile_field_value_key_cardinality,
    ADD CONSTRAINT contact_profile_field_value_key_cardinality
        CHECK ((field = 'phone') = (value_key <> '')) NOT VALID;
