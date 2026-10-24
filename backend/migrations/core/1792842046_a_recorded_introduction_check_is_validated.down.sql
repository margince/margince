-- A validated CHECK has no ALTER that returns it to NOT VALID. Dropping it
-- belongs to the migration that added it.
SELECT 1;
