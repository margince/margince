-- Validate the check the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in ONE transaction: run in
-- the same one, this scan would sit under the ACCESS EXCLUSIVE that ALTER holds
-- to commit, and the SHARE UPDATE EXCLUSIVE that VALIDATE asks for would be
-- nothing but a weaker lock requested while a stronger one is held. Here it is
-- the only statement in its own transaction, so the scan really does run under
-- the lighter lock and brief writes do not queue behind it.
--
-- It cannot fail: the column was created in that migration with a constant
-- default, so every row it scans carries ''.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run VALIDATE CONSTRAINT brief_run_revenue_norm_currency_check;
