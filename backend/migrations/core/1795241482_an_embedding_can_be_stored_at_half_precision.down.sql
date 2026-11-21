-- Run by hand on a rollback, when the app is not busy. The first statement
-- takes ACCESS EXCLUSIVE and holds it to the end, so every reader and writer of
-- embedding waits while each vector is written back. lock_timeout bounds only
-- the wait for that lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE embedding DROP CONSTRAINT embedding_holds_one_vector;
UPDATE embedding SET embedding = embedding_half::vector WHERE embedding IS NULL;
ALTER TABLE embedding ALTER COLUMN embedding SET NOT NULL;
ALTER TABLE embedding DROP COLUMN embedding_half;
