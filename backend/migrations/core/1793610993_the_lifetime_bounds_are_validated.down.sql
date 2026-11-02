-- A constraint cannot be un-validated, so this reverts nothing. The bounds
-- themselves are dropped by the down of the migration that added them, the
-- a_token_expires_after_it_was_created pair.
SET LOCAL lock_timeout = '3s';
