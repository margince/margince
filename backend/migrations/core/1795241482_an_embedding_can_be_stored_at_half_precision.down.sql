SET LOCAL lock_timeout = '3s';

ALTER TABLE embedding DROP CONSTRAINT embedding_holds_one_vector;
UPDATE embedding SET embedding = embedding_half::vector WHERE embedding IS NULL;
ALTER TABLE embedding ALTER COLUMN embedding SET NOT NULL;
ALTER TABLE embedding DROP COLUMN embedding_half;
