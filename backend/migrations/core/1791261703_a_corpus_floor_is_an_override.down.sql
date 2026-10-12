SET LOCAL lock_timeout = '5s';

UPDATE knowledge_corpus SET min_similarity = 0.35 WHERE min_similarity IS NULL;
ALTER TABLE knowledge_corpus ALTER COLUMN min_similarity SET DEFAULT 0.35;
ALTER TABLE knowledge_corpus ALTER COLUMN min_similarity SET NOT NULL;
