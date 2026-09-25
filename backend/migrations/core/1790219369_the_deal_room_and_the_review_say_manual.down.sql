-- Deliberately empty. Once two words are one word the row no longer says which
-- it was, and a down migration that guessed would be worse than one that does
-- nothing: it would invent a distinction rather than restore one.
SELECT 1;
