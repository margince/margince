-- Seeded rep and manager roles move from team scope to own scope.
--
-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so a change to the
-- compiled defaults alone would take effect on a FRESH database and nowhere
-- else — every deployed installation would keep team scope permanently, and
-- the two would diverge with nothing to show it.
--
-- Guarded three ways, each for its own reason:
--   is_system      — an operator's custom role that happens to be keyed 'rep'
--                    is theirs, not ours to rewrite.
--   row_scope=team — idempotent, and it leaves alone an operator who has
--                    already chosen a different scope for the seeded role.
--   the key set    — only the two roles whose default changed.
UPDATE role
   SET permissions = jsonb_set(permissions, '{row_scope}', '"own"'::jsonb, true)
 WHERE key IN ('rep', 'manager')
   AND is_system
   AND permissions ->> 'row_scope' = 'team';
