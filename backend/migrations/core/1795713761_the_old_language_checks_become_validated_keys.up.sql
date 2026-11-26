SET LOCAL lock_timeout = '3s';

-- A member's language and an activity's language were held by a CHECK on the
-- three shipped codes, so no stored row can fail the new key. The other six
-- keys stay NOT VALID: their columns accepted any text, and the
-- unknown_language_values data task lists the rows that would fail.
ALTER TABLE activity VALIDATE CONSTRAINT activity_language_fkey;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_locale_fkey;
