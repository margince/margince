SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS tag_suggestion_evidence;
DROP TABLE IF EXISTS tag_suggestion;
ALTER TABLE tag DROP CONSTRAINT IF EXISTS tag_suggestible_is_described;
ALTER TABLE tag DROP COLUMN IF EXISTS suggestible;
