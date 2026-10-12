-- A validated constraint cannot be made NOT VALID again, and the previous
-- migrations' down files drop it with its columns.
SELECT 1;
