-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so an object added to
-- the compiled defaults alone is granted to nobody who bootstrapped earlier — it
-- works on a fresh database and 403s everywhere else, permanently. The grants
-- below are the same ones the compiled defaults carry.
SET LOCAL lock_timeout = '5s';

-- Planning a week and settling it is what every seat with a week does. The
-- delete is the one verb that differs: an admin or an operator may remove a
-- plan outright, a rep may not — a week that was planned is a thing that
-- happened, and a commitment is dropped rather than erased.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,weekly_plan}',
        '{"create": true, "read": true, "update": true, "delete": true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops', 'management', 'manager')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'weekly_plan';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,weekly_plan}',
        '{"create": true, "read": true, "update": true, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key = 'rep'
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'weekly_plan';

-- A read-only seat reads a plan and writes none.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,weekly_plan}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key = 'read_only'
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'weekly_plan';
