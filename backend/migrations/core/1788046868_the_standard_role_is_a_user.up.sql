-- The standard seat's role reads as "User" everywhere, including here.
--
-- `role.name` is a display string an installation carries in its own rows, and
-- two surfaces render it verbatim: the extension access matrix and any screen
-- that lists roles by name. The roster renders the i18n catalog instead, so
-- leaving this row alone would have printed "User" and "Member" for one role on
-- one page.
--
-- Scoped to the SEEDED row, not to every role whose name happens to match: a
-- workspace that authored its own role called "Member" meant its own thing by
-- it, and renaming that would be editing somebody's data to win an argument
-- about vocabulary.
SET LOCAL lock_timeout = '3s';
UPDATE role SET name = 'User', updated_at = now()
 WHERE key = 'rep' AND is_system AND name = 'Member';
