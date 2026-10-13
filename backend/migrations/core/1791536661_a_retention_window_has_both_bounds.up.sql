-- retain_days had no CHECK, so the store was its only guard.
--
-- A zero erases records at the moment they are created, and a value past two
-- million days makes `now() - make_interval(days => retain_days)` answer
-- "timestamp out of range" instead of acting. The sibling column `action`
-- already carries a CHECK, so the table knows the pattern.
--
-- NOT VALID: the table is written by a live product, and an existing row
-- outside the bounds must not fail this migration. The matching VALIDATE runs
-- in its own file.
-- Bounded so an open transaction holding a conflicting lock stalls this
-- migration rather than every write to the table.
SET LOCAL lock_timeout = '3s';

ALTER TABLE retention_policy
	ADD CONSTRAINT retention_policy_retain_days_check
	CHECK (retain_days >= 1 AND retain_days <= 36500) NOT VALID;
