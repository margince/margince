-- Put the seeded manager role back to own scope — but only where THIS
-- migration is what widened it.
--
-- row_scope_set_by is the provenance the up migration wrote. Without it this
-- rollback could not tell a role it moved to team scope from one an operator
-- had already chosen to put there, because afterwards the two documents look
-- the same. Narrowing the second kind would withdraw write authority the
-- operator granted deliberately, and nothing would say it had happened.
--
-- So an installation where the up migration matched nothing — an operator had
-- already moved the seeded role off `own` — passes through this untouched.
--
-- The marker goes with the scope, so the pair is reversible in both directions:
-- reapplying the up migration afterwards finds `own` again and widens again.
UPDATE role
   SET permissions = jsonb_set(permissions, '{row_scope}', '"own"'::jsonb, true)
                     - 'row_scope_set_by'
 WHERE key = 'manager'
   AND is_system
   AND permissions ->> 'row_scope' = 'team'
   AND permissions ->> 'row_scope_set_by' = '1788244324';
