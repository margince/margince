-- Dropping the fetch columns, and putting back what the up migration moved.
--
-- The up migration cleared opened_at on rows whose value it could not
-- attribute, moving it to first_fetched_at. Coming back down, first_fetched_at
-- is about to disappear — so a row that was cleared would lose the moment
-- entirely, which is worse than the ambiguity the up migration resolved. The
-- value returns to where it was, meaning exactly what it meant before.
SET LOCAL lock_timeout = '3s';

UPDATE confirm_token
   SET opened_at = first_fetched_at
 WHERE opened_at IS NULL AND first_fetched_at IS NOT NULL;

ALTER TABLE confirm_token
    DROP COLUMN IF EXISTS first_fetched_at,
    DROP COLUMN IF EXISTS fetch_count;
