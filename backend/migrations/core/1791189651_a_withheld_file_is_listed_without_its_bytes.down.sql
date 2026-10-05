-- Rows written without bytes stay, as rows with an empty storage key; partslim
-- already skips those, and nothing else wrote one before this column.
SET LOCAL lock_timeout = '3s';
ALTER TABLE attachment DROP COLUMN IF EXISTS bytes_withheld;
