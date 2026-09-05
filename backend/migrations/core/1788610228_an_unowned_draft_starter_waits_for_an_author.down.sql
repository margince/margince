-- Re-enabling an unowned draft would restore a configuration that cannot run.
-- A downgrade preserves the pause; an authorized person may enable it later.
SET LOCAL lock_timeout = '3s';
