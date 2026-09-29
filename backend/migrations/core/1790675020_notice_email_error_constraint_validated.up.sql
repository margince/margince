-- The second half of the notice email-attempt constraint, and it is a file of
-- its own for one reason: this runner gives each migration ONE transaction.
--
-- The constraint was added NOT VALID two migrations back, which took ACCESS
-- EXCLUSIVE only long enough to record it and scanned nothing. Validating it in
-- that same script would have run the scan under that same lock, still held —
-- a transaction does not release a lock because a later statement wants a
-- weaker one. Here, in its own transaction, VALIDATE takes SHARE UPDATE
-- EXCLUSIVE, which readers and writers both pass.
--
-- It cannot fail. Both columns were created by the migration that added this
-- constraint, so every row that predates it carries NULL in each, and
-- `email_error IS NULL` satisfies the predicate. The scan is run anyway rather
-- than leaving the constraint NOT VALID: the planner does not trust an
-- unvalidated constraint, and it reads as provisional to the next person who
-- looks at the schema.
SET LOCAL lock_timeout = '3s';

ALTER TABLE notice VALIDATE CONSTRAINT notice_email_error_after_attempt;
