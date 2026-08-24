-- Put the seeded rep and manager roles back to team scope.
--
-- It is NOT the exact inverse, and cannot be. This migration cannot tell a role
-- the up migration moved to `own` from one an operator chose to put there, so a
-- rollback widens the second kind along with the first — from owner-only to
-- every teammate's records. That is a widening, which is the direction a
-- rollback must never take silently.
--
-- So it is guarded to the case it can reason about: only the two system roles,
-- and only where the document still looks exactly as the up migration left it.
-- An operator who has since edited the role at all is left alone, and an
-- installation that reaches this migration with its own deliberate `own` scope
-- keeps it.
--
-- A rollback past this point also needs 1787449900 rolled back with it, or the
-- installation lands on own-scoped reps WITH the deal-amount mask restored —
-- amounts hidden on every deal a rep does not personally own, which is the
-- state neither the old model nor the new one has.
UPDATE role
   SET permissions = jsonb_set(permissions, '{row_scope}', '"team"'::jsonb, true)
 WHERE key IN ('rep', 'manager')
   AND is_system
   AND permissions ->> 'row_scope' = 'own';
