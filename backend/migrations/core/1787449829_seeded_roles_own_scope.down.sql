-- Put the seeded rep and manager roles back to team scope, under the same
-- guards the up migration used: only the system roles, and only where they
-- still carry the scope that migration set.
UPDATE role
   SET permissions = jsonb_set(permissions, '{row_scope}', '"team"'::jsonb, true)
 WHERE key IN ('rep', 'manager')
   AND is_system
   AND permissions ->> 'row_scope' = 'own';
