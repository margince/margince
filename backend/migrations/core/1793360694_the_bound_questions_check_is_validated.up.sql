-- Validate the check the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in one transaction. Here the
-- VALIDATE is the only statement, so its scan runs under SHARE UPDATE EXCLUSIVE
-- and brief writes do not queue behind the ALTER's ACCESS EXCLUSIVE.
--
-- It cannot fail: the column was created with the constant default '{}'.
SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition_revision VALIDATE CONSTRAINT report_definition_revision_question_bindings_is_an_object;
