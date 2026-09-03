-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so an object added to
-- the compiled defaults alone is granted to nobody who bootstrapped earlier — it
-- works on a fresh database and 403s everywhere else, permanently.
--
-- Only ONE population needs reaching here: `forecast` has never existed, so no
-- role holds the key and every role that should have it is missing it. The
-- guard is therefore absence, and an operator who has since hand-set the object
-- keeps their setting. (The other population — a role holding the object with
-- the wrong verbs — has no members for a brand-new object, and a guard written
-- for it alone would match nobody.)
SET LOCAL lock_timeout = '5s';

-- Calling the number is a manager's job: an assertion about a team's forecast,
-- made by whoever is accountable for it.
--
-- No update and no delete for any seat. A reading is DERIVED, so there is
-- nothing to edit, and a current call SUPERSEDES rather than being rewritten —
-- the chain of what was believed when is the record. A grant for a verb the
-- product does not offer reads as an oversight the next author has to research.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,forecast}',
        '{"create": true, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops', 'management', 'manager')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'forecast';

-- A rep reads the forecast their own pipeline is in, and calls none: a rep
-- asserting a number would be calling a figure they are not accountable for.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,forecast}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key IN ('rep', 'read_only')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'forecast';
