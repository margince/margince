-- The seeded manager (Team Lead) role moves from own scope to team scope.
--
-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so a change to the
-- compiled defaults alone would take effect on a FRESH database and nowhere
-- else — every deployed installation would keep own scope permanently, and the
-- two would diverge with nothing to show it.
--
-- This is a WIDENING: a Team Lead gains the manager grid over the records of
-- everyone sharing a live team with them, where before they reached only their
-- own. The product ruling is that a Team Lead manages their team, so they read
-- and work their team's records without an explicit share being arranged first.
-- A record_grant naming a team keeps working and is now redundant for this
-- seat; it stays the mechanism for every other case.
--
-- Guarded three ways, each for its own reason:
--   is_system     — an operator's custom role that happens to be keyed
--                   'manager' is theirs, not ours to rewrite.
--   row_scope=own — idempotent, and it leaves alone an operator who has
--                   already chosen a different scope for the seeded role.
--   the key       — 'rep' stays own-scoped; only the Team Lead's default moved.
--
-- row_scope_set_by records that THIS migration made the change, so the down
-- migration can tell a role it widened from one an operator had already put at
-- team scope themselves. Matching on the current value alone cannot: both look
-- identical afterwards, and a rollback would narrow a setting it never made.
-- The marker is removed by the down migration, so a rolled-back-then-reapplied
-- installation is in the same state as one that only ever ran the up.
UPDATE role
   SET permissions = jsonb_set(
         jsonb_set(permissions, '{row_scope}', '"team"'::jsonb, true),
         '{row_scope_set_by}', '"1788244324"'::jsonb, true)
 WHERE key = 'manager'
   AND is_system
   AND permissions ->> 'row_scope' = 'own';
