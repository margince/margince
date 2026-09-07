-- Narrow the admitted set back to de and en.
--
-- This REFUSES while any row records Vietnamese rather than quietly clearing
-- them. A detected language is an observation somebody's search results now
-- depend on: nulling it rewrites that row's search_tsv back to the unstemmed
-- dictionary, and a rollback that silently changes what lexical search finds is
-- worse than one that stops and says so.

-- A bounded wait: the ALTER takes a lock that blocks writers on activity, and
-- an open transaction holding a conflicting one would otherwise stall every
-- write to the table for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM activity WHERE language = 'vi') THEN
        RAISE EXCEPTION
            'activity rows record Vietnamese; narrowing the language check would have to erase observations that search results depend on';
    END IF;
END $$;

ALTER TABLE activity DROP CONSTRAINT activity_language_check;

ALTER TABLE activity ADD CONSTRAINT activity_language_check
    CHECK (language IS NULL OR language IN ('de', 'en'));
