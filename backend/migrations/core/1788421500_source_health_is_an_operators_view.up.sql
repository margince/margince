-- Who may read the installation's own source health.
--
-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so an object added to
-- the compiled defaults alone is granted to nobody who bootstrapped earlier — it
-- works on a fresh database and 403s everywhere else, permanently.
--
-- One population needs reaching: `data_coverage` has never existed, so no role
-- holds the key and the guard is absence. An operator who later hand-sets the
-- object keeps their setting.
SET LOCAL lock_timeout = '5s';

-- Admin and ops read source health because it is their job: a connector that
-- stopped, a mailbox the installation may not read, an import that never
-- finished. Read only — coverage is OBSERVED, and there is nothing on this
-- surface for anybody to write.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,data_coverage}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'data_coverage';

-- Everyone else: nothing, and the absence is the point. A rep or a manager
-- shown their installation's connector health has been handed somebody else's
-- job — and a screen reporting a problem they cannot act on teaches them to
-- ignore the screen.
--
-- Written as an explicit zero grant rather than left out. A role with no key at
-- all is indistinguishable from a role the backfill missed, and the next reader
-- auditing this cannot tell a decision from an omission.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,data_coverage}',
        '{"create": false, "read": false, "update": false, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'rep', 'read_only')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'data_coverage';
