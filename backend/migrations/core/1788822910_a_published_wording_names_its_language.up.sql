-- A published wording is unique per LANGUAGE, not per key and version alone.
--
-- The controller mail now speaks the installation's own language, so one
-- template at one version has an English, a German and a Vietnamese text. The
-- old index admitted exactly one of the three and the second publish collided,
-- which would have failed the boot that published them.
--
-- locale already existed on this table, documented as "NULL means everywhere".
-- That reading is unchanged: a wording narrowed to a language names it, and one
-- that applies everywhere still names none. NULLS NOT DISTINCT is what keeps
-- the everywhere-row unique against itself, which a plain unique index would
-- not — under the default, two NULL locales are distinct and both would be
-- admitted.
-- Bounded, so a transaction holding a conflicting lock stalls this migration
-- rather than every write to the table behind it.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS consent_text_version_published;

CREATE UNIQUE INDEX consent_text_version_published
    ON consent_text_version (key, version, locale) NULLS NOT DISTINCT
    WHERE published_at IS NOT NULL;
