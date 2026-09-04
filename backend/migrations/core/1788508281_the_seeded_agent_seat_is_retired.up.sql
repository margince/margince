-- Retire the seeded agent seat: it costs a licence seat and authorizes nothing.
--
-- Bootstrap wrote every installation one `is_agent` app_user to give the
-- extension-job dispatcher a non-zero id to record. The tick now answers as the
-- JOB it is — no user, no seat, no permissions — so nothing reads this row. What
-- it still does is occupy a full licence seat, appear in the admin roster as an
-- identity an admin cannot act on correctly, and read as an off switch for work
-- it does not gate.
--
-- DEACTIVATE AND ARCHIVE; NEVER DELETE, for two independent reasons.
--
-- A DELETE can fail the deploy. Dozens of columns reference `app_user(id)` and
-- many of them refuse the delete outright — RESTRICT explicitly, or NO ACTION by
-- omission, which blocks it just the same. `channel_connection.connected_by`,
-- `passport.granted_by` and `scheduled_send.scheduled_by` are among them. Those
-- columns are actor-derived and this row can never be an actor, so the product
-- cannot write one; an operator or a repair script can, and a runner seeded at
-- this address later certainly could.
--
-- And a DELETE destroys evidence, which holds even with no referencing row
-- anywhere: `audit_log.actor_id` is plain text with no FK, so deleting the row
-- leaves audit entries naming an id nothing can resolve.
--
-- BOTH COLUMNS DO WORK. The licence meter filters on `status NOT IN
-- ('suspended','deactivated')` and never reads `archived_at`, so archiving alone
-- frees no seat; archiving is what drops the row from the roster and from every
-- live-member predicate.
--
-- THE PREDICATE names the row bootstrap wrote and nothing else. `password_hash
-- IS NULL` plus the reserved `.gradion.local` domain identify it; an
-- installation that has since minted a real agent identity keeps it. Widening
-- this would deactivate a runner somebody is using.
--
-- The last two clauses are what make the DOWN a true inverse. Every row this
-- statement touches held exactly (`active`, NULL), so restoring exactly that is
-- correct for exactly those rows — no prior state has to be recorded anywhere.
-- A seat an operator had already deactivated or archived is skipped, which costs
-- nothing: it is already out of the meter and out of the roster, which is all
-- this migration is for.
--
-- The two CHECK constraints stay. `is_agent` remains a supported column —
-- `overlay`'s mappable-seat predicate and `federatedidentity`'s sign-in refusal
-- both filter on it, and a resident runner will land under it. In particular
-- `app_user_agent_is_full` is what makes "an agent seat is always counted" true
-- rather than aspirational: identity/seatusage.go cites it by name as the reason
-- the meter includes agents, so dropping it would let an agent be a read seat
-- and silently unmeter it.
SET LOCAL lock_timeout = '3s';

UPDATE app_user
   SET status = 'deactivated',
       archived_at = now()
 WHERE is_agent
   AND password_hash IS NULL
   AND email LIKE 'agent@%.gradion.local'
   AND status = 'active'
   AND archived_at IS NULL;
