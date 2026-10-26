-- Validate the check the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in one transaction. Run in
-- the same one, the scan would sit under the ACCESS EXCLUSIVE that the ALTER
-- holds to commit.
--
-- It cannot fail: the check only gained a value, so every existing row passes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE retention_policy VALIDATE CONSTRAINT retention_policy_action_check;
