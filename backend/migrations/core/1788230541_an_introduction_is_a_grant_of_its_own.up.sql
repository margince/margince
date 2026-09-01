-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so an object added to
-- the compiled defaults alone is granted to nobody who bootstrapped earlier — it
-- works on a fresh database and 403s everywhere else, permanently. The grants
-- below are the same ones the compiled defaults carry.
SET LOCAL lock_timeout = '5s';

-- Asking a colleague to open a door, and answering an ask made of you, are both
-- the job. The grant admits a seat to the surface; WHICH of the two parties you
-- are on a given row is the row's own check, and no grant stands in for it.
-- No delete: an ask that was made is a thing that happened, so it is withdrawn
-- rather than erased.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,introduction}',
        '{"create": true, "read": true, "update": true, "delete": false}'::jsonb, true)
    WHERE key IN ('admin', 'ops', 'manager', 'management', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'introduction';

-- A read-only seat sees that an introduction is in flight — it is part of what
-- is happening around the contact they are reading — and asks for nothing.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,introduction}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE key = 'read_only'
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'introduction';
