SET LOCAL lock_timeout = '3s';
-- One row per field again: each contact keeps its most recently observed number.
DELETE FROM contact_profile_field f
 USING contact_profile_field newer
 WHERE f.field = 'phone' AND newer.field = 'phone' AND newer.contact_id = f.contact_id
   AND (newer.observed_at, newer.id) > (f.observed_at, f.id);

ALTER TABLE contact_profile_field
    DROP CONSTRAINT contact_profile_field_value_key_cardinality,
    DROP CONSTRAINT uq_contact_profile_field,
    ADD CONSTRAINT uq_contact_profile_field UNIQUE (contact_id, field);
UPDATE contact_profile_field SET value_key = '' WHERE value_key <> '';
