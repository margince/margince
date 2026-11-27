SET LOCAL lock_timeout = '3s';

-- A member's language was held by a CHECK on the three shipped codes, so no
-- stored row can fail the new key, and app_user is a small table. The activity
-- key stays NOT VALID: its table is the largest in the schema, and the same
-- old CHECK already kept every stored row inside the three codes. The other
-- six keys stay NOT VALID too, because their columns took any text, and the
-- unknown_language_values data task lists the rows that would fail.
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_locale_fkey;
