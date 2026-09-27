SET LOCAL lock_timeout = '3s';
-- A contact's phone is a list, so its evidence becomes one row per number.
-- value_key tells two rows of one field apart: the E.164 number for a phone,
-- empty for every single-answer field.
--
-- Only the column here, with a constant default, so the ACCESS EXCLUSIVE this
-- takes is a catalog change and no rewrite. The backfill and the key swap are
-- the next migration, which never takes that lock until its last statement.
ALTER TABLE contact_profile_field ADD COLUMN value_key text NOT NULL DEFAULT '';
