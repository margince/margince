-- Return the seeded agent seat to the state the up migration found it in.
--
-- This is a TRUE INVERSE, and the up migration is what makes it one: it touches
-- only rows that held exactly (`active`, NULL), so restoring exactly that pair
-- restores exactly what was there. Nothing has to be recorded to roll this back.
--
-- The `deactivated` and `archived_at IS NOT NULL` clauses keep this to rows in
-- the state the up leaves behind. Deactivating and archiving are independent
-- here, so a seat an operator merely deactivated — which the up skipped — is
-- skipped by this too, and their decision survives the rollback.
--
-- ONE CASE IS GENUINELY AMBIGUOUS: a seat an operator deactivated AND archived
-- by hand is indistinguishable from one this migration retired, so this
-- statement reactivates it. Separating them would mean the up recording prior
-- state somewhere, which is a permanent table on every installation to serve the
-- rollback of a row nothing reads. The failure direction is the safe one — the
-- seat comes back live but inert, holding no password and no role, with no
-- passport able to name it — so it costs one metered seat and a roster row.
SET LOCAL lock_timeout = '3s';

UPDATE app_user
   SET status = 'active',
       archived_at = NULL
 WHERE is_agent
   AND password_hash IS NULL
   AND email LIKE 'agent@%.gradion.local'
   AND status = 'deactivated'
   AND archived_at IS NOT NULL;
