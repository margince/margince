-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so an object added to
-- the compiled defaults alone is granted to nobody who bootstrapped earlier — it
-- works on a fresh database and 403s everywhere else, permanently. The grants
-- below are the same ones the compiled defaults carry.
SET LOCAL lock_timeout = '5s';

-- Naming things is an ownership act. A tag is a shared vocabulary: the set is
-- small, everyone sorts by it, and a second spelling of one that already exists
-- splits every list that reads it — so creating and retiring them belongs to the
-- seats that already hold the installation's configuration.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,tag}',
        '{"create": true, "read": true, "update": true, "delete": true}'::jsonb, true)
    WHERE key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'tag';

-- Read is what every other seat needs and all it needs. Applying a tag to a
-- record is governed by the write on the RECORD, not by authority over the
-- vocabulary — so a rep who may edit a deal may tag it, and still cannot invent
-- a tag the whole installation then sees.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,tag}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE key IN ('manager', 'management', 'rep', 'read_only')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'tag';
