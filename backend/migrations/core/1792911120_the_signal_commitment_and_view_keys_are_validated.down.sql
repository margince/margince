-- A validated constraint cannot be made NOT VALID again, and the earlier
-- migrations' down files drop it with its columns, so there is nothing to undo.
SET LOCAL lock_timeout = '3s';
