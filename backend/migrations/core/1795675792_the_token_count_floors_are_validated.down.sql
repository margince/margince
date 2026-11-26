-- A constraint cannot be un-validated, so this reverts nothing. The down of
-- the migration that added the floors drops them.
SET LOCAL lock_timeout = '3s';
