SET LOCAL lock_timeout = '3s';
-- Key every stored phone row by the number the writer derives, then make that
-- key part of the uniqueness, so a contact can hold one evidence row per number.
--
-- The lock order is what keeps readers moving. The UPDATE touches only phone
-- rows, at most one per contact until now, under ROW EXCLUSIVE. The index is
-- built under SHARE, which lets reads through while writes wait. Only the final
-- ALTER takes ACCESS EXCLUSIVE, and it swaps a built index in without a scan
-- and adds the check NOT VALID, so it is held for the commit and no longer.
--
-- The key is the writer's (contacts.profileFieldValueKey): values.ParsePhone's
-- E.164 form, or the value trimmed as strings.TrimSpace trims it when the
-- number does not parse. The trim set is Go's unicode.IsSpace, spelled out,
-- because btrim's default trims the ASCII space alone and a key that differs
-- by one invisible character never meets its next observation or its restore.
WITH keyed AS (
    SELECT id, btrim(value, E' \t\n\x0B\f\r\u0085                 　') AS trimmed
      FROM contact_profile_field
     WHERE field = 'phone'
)
UPDATE contact_profile_field f
   SET value_key = COALESCE(
         substring(regexp_replace(regexp_replace(k.trimmed, '[ ()./\t-]', '', 'g'), '^00', '+')
                   FROM '^\+[1-9][0-9]{7,14}$'),
         NULLIF(k.trimmed, ''),
         f.id::text)
  FROM keyed k
 WHERE f.id = k.id;

CREATE UNIQUE INDEX uq_contact_profile_field_by_value
    ON contact_profile_field (contact_id, field, value_key);

ALTER TABLE contact_profile_field
    DROP CONSTRAINT uq_contact_profile_field,
    ADD CONSTRAINT uq_contact_profile_field UNIQUE USING INDEX uq_contact_profile_field_by_value,
    ADD CONSTRAINT contact_profile_field_value_key_cardinality
        CHECK ((field = 'phone') = (value_key <> '')) NOT VALID;
