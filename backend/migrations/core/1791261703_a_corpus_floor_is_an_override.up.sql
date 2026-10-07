SET LOCAL lock_timeout = '5s';

-- NULL means "the binding's measured floor"; a number is the workspace's own
-- override. Every row at 0.35 took the old column default and was never moved,
-- so it carries no choice to preserve.
ALTER TABLE knowledge_corpus ALTER COLUMN min_similarity DROP NOT NULL;
ALTER TABLE knowledge_corpus ALTER COLUMN min_similarity DROP DEFAULT;
UPDATE knowledge_corpus SET min_similarity = NULL WHERE min_similarity = 0.35;
