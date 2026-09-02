-- Tags reach the roles an installation already had.
--
-- 1788320100 made `tag` governed vocabulary and NARROWED the roles that held
-- write on it. What it could not do is ADD the object to a role that never had
-- it: its guard is `(permissions -> 'objects') ? 'tag'`, which is the right
-- guard for a narrowing and the wrong one for an installation bootstrapped
-- before tag was a governed object at all. Those roles carry no `tag` key, so
-- every tag route answers 403 for them — permanently, since seedSystemRoles
-- writes each document once at workspace creation and never re-syncs.
--
-- The verbs are the seeded matrix's own, per role: Admin and Ops administer the
-- vocabulary, everyone else reads it and applies what is there. Adding read to
-- the roles that only apply tags is not a widening — applying one is what the
-- taggable surface is for, and a role that cannot READ the vocabulary cannot
-- see the tags already on the records it works.
--
-- GUARDED ON ABSENCE, so it is a no-op for an installation that has the key
-- (including one an operator has narrowed by hand, and one 1788320100 has
-- already corrected), and re-running changes nothing.
--
-- lock_timeout because `role` is a pre-existing table: a deploy must fail fast
-- rather than queue behind a long reader.
SET LOCAL lock_timeout = '3s';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,tag}',
        '{"create": true, "read": true, "update": true, "delete": true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT ((permissions -> 'objects') ? 'tag');

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,tag}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'rep', 'read_only')
      AND permissions ? 'objects'
      AND NOT ((permissions -> 'objects') ? 'tag');
