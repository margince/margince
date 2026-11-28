-- A constraint cannot be un-validated, so this reverts nothing. The down of
-- the migration that rebuilt the check puts the old one back.
SET LOCAL lock_timeout = '3s';
