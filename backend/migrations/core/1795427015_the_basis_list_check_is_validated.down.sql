-- A dropped constraint is not restored: it would break the rows that made the
-- upgrade drop it. The rollback of the migration before this one restores the
-- old lists.
SELECT 1;
