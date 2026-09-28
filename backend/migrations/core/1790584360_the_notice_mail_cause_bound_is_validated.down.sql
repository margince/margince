-- Nothing to undo, for the reason the sibling validation states: a validated
-- CHECK cannot be returned to NOT VALID, and dropping and re-adding it would
-- take the ACCESS EXCLUSIVE lock the pair exists to avoid. The constraint goes
-- with the migration that added it.
SELECT 1;
