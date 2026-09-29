-- The second half of the notice mail-cause bound, and it is a file of its own
-- because this runner gives each migration ONE transaction: validating in the
-- file that added the constraint would run the scan under the ACCESS EXCLUSIVE
-- that ALTER took and still holds. Here VALIDATE takes SHARE UPDATE EXCLUSIVE,
-- which readers and writers both pass.
--
-- It cannot fail. Every row carries NULL in email_error unless a send already
-- failed, and the writer truncates to the same 500 before it stores one. The
-- scan is run anyway rather than leaving the constraint NOT VALID: the planner
-- does not trust an unvalidated constraint, and it reads as provisional to the
-- next person who looks at the schema.
SET LOCAL lock_timeout = '3s';

ALTER TABLE notice VALIDATE CONSTRAINT notice_email_error_bounded;
