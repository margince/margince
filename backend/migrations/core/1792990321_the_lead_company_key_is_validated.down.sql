-- A validated constraint cannot be made NOT VALID again, and the earlier
-- migration's down file drops it with its column, so there is nothing to undo.
SET LOCAL lock_timeout = '3s';
