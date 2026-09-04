-- Return the seeded agent seat to the state the up migration found it in.
--
-- It restores the pair the up migration's subject held: `active`, un-archived.
--
-- The `deactivated` and `archived_at IS NOT NULL` clauses keep this to rows in
-- the state the up leaves behind. Deactivating and archiving are independent
-- here, so a seat an operator merely deactivated — which the up skipped — is
-- skipped by this too, and their decision survives the rollback.
--
-- TWO CASES ARE NOT PERFECTLY INVERTED, both for one reason: nothing records
-- what a row held before. A seat an operator deactivated AND archived by hand is
-- indistinguishable from one this migration retired, so this reactivates it; and
-- a seat that was already archived while still ACTIVE comes back un-archived,
-- because the up preserved that timestamp and this cannot tell it from its own.
--
-- Separating them means the up writing prior state somewhere — a permanent table
-- on every installation, to serve the rollback of a row nothing reads. The
-- failure direction is the safe one instead: the seat returns live but inert,
-- holding no password and no role, with no passport able to name it, so the cost
-- is one metered seat and a roster row until somebody archives it again.
SET LOCAL lock_timeout = '3s';

UPDATE app_user
   SET status = 'active',
       archived_at = NULL
 WHERE is_agent
   AND password_hash IS NULL
   AND email LIKE 'agent@%.gradion.local'
   AND status = 'deactivated'
   AND archived_at IS NOT NULL;
