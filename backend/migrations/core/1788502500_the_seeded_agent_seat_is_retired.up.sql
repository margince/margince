-- Retire the seeded agent seat: it costs a licence seat and authorizes nothing.
--
-- Bootstrap wrote every installation one `is_agent` app_user whose entire
-- production job was to give the extension-job dispatcher a non-zero id to
-- record. The tick now answers as the JOB it is — no user, no seat, no
-- permissions — so nothing in the running product reads this row. What it still
-- does is occupy a full licence seat on every installation, appear in the admin
-- roster as an identity somebody might try to act on, and read as an operator's
-- off switch for work it does not gate.
--
-- DEACTIVATE AND ARCHIVE; NEVER DELETE. `app_user(id)` is referenced 92 times,
-- 16 of them ON DELETE RESTRICT — `channel_connection.connected_by`,
-- `passport.granted_by`, `scheduled_send.scheduled_by`, `capture_import.user_id`
-- and `custom_field.created_by` among them. A connector configured against the
-- seat names it directly, so a seat-referencing row is not hypothetical, and a
-- DELETE would fail the deploy on the first installation that has one.
--
-- DEACTIVATED is what does the work, not archived. `identity`'s licence meter
-- filters on `status NOT IN ('suspended','deactivated')` and never reads
-- `archived_at`, so archiving alone would free nothing. Archiving is what drops
-- the row from the roster and from every live-member predicate. Both, therefore.
--
-- The PREDICATE names the row bootstrap wrote and nothing else. An installation
-- that has since minted a real agent identity — one with a password, or under a
-- different address — keeps it: `password_hash IS NULL` and the reserved
-- `.gradion.local` domain together identify the seeded row, and a first-party
-- runner that lands later is not this row and must not be caught by this.
--
-- The two CHECK constraints STAY, against an earlier draft of this change that
-- dropped them. `is_agent` remains a supported column: `overlay`'s mappable-seat
-- predicate and `federatedidentity`'s sign-in refusal both filter on it, tests
-- seed their own agent rows, and a resident runner will land under it.
-- `app_user_agent_is_full` is what makes "an agent seat is always counted" true
-- rather than aspirational — identity/seatusage.go cites it by name as the
-- reason the meter includes agents. Dropping it would let an agent be a read
-- seat and silently unmeter it.
--
-- 1788129035_the_agent_seat_is_margince.* stays too. A shipped migration is
-- additive-only; its subject being retired does not make it removable, and
-- editing one changes what FRESH installations get while deployed databases
-- keep the old behaviour.
SET LOCAL lock_timeout = '3s';

UPDATE app_user
   SET status = 'deactivated',
       archived_at = COALESCE(archived_at, now())
 WHERE is_agent
   AND password_hash IS NULL
   AND email LIKE 'agent@%.gradion.local';
