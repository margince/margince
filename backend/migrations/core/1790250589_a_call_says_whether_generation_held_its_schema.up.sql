-- An adapter whose vendor cannot enforce a response schema as written now
-- decides so before sending: it moves the bounds the decoder cannot hold into
-- descriptions ('relaxed'), sends the whole schema without asking the endpoint
-- to enforce it ('unenforced', an OpenAI-wire strict:false), or sends no schema
-- at all when no form of it fits ('dropped'). The answer is still validated
-- against the whole schema, but a reply generation did not constrain fails that
-- validator for a different reason than one it did, and without this column
-- the two rows are identical. Empty is the ordinary case: the schema went as
-- written, or there was none.
--
-- The CHECK is added NOT VALID and validated by the next migration, in a file
-- of its own. Written inline on the ADD COLUMN it would scan every ai_call row
-- under the ACCESS EXCLUSIVE lock the ALTER takes, and the runner holds that
-- lock until the file commits, so a VALIDATE in this same file would scan under
-- it too. Every existing row satisfies the check trivially: the column is new,
-- so they all carry the default. The split is about the lock, not about doubt.
--
-- ALTER TABLE takes a lock that blocks writers on a table this migration did
-- not create, so the wait is bounded: without a timeout, one open transaction
-- holding a conflicting lock stalls every ai_call write for as long as this
-- migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  ADD COLUMN schema_downgrade text DEFAULT ''::text NOT NULL,
  ADD CONSTRAINT ai_call_schema_downgrade_check
    CHECK (schema_downgrade = ANY (ARRAY[''::text, 'relaxed'::text, 'unenforced'::text, 'dropped'::text])) NOT VALID;
