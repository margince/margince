SET LOCAL lock_timeout = '3s';
-- A contact's phone is a list, so its evidence is one row per number. One row
-- per field made a signature's four numbers overwrite each other, and each
-- write recorded the previous number as replaced while both were current.
-- value_key tells two rows of one field apart: the E.164 number for a phone,
-- empty for every single-answer field.
ALTER TABLE contact_profile_field ADD COLUMN value_key text NOT NULL DEFAULT '';

-- The key the writer derives (values.ParsePhone), so a later statement of the
-- same number lands on this row instead of beside it. A value that does not
-- normalize keys on itself, as the writer does.
UPDATE contact_profile_field
   SET value_key = COALESCE(
         substring(regexp_replace(regexp_replace(value, '[ ()./\t-]', '', 'g'), '^00', '+')
                   FROM '^\+[1-9][0-9]{7,14}$'),
         NULLIF(value, ''),
         id::text)
 WHERE field = 'phone';

ALTER TABLE contact_profile_field DROP CONSTRAINT uq_contact_profile_field;
ALTER TABLE contact_profile_field
    ADD CONSTRAINT uq_contact_profile_field UNIQUE (contact_id, field, value_key);
ALTER TABLE contact_profile_field
    ADD CONSTRAINT contact_profile_field_value_key_cardinality CHECK ((field = 'phone') = (value_key <> ''));
