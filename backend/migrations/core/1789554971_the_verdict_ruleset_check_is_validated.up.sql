-- The scan the migration before this one deliberately did not run.
--
-- Its ALTER took ACCESS EXCLUSIVE to record the constraint and committed,
-- releasing it. VALIDATE takes SHARE UPDATE EXCLUSIVE, which readers and
-- writers pass, so the scan happens with the table still serving. That
-- separation is the whole reason the two statements are in two files:
-- dbmigrate runs a file in one transaction, so a VALIDATE beside its own
-- ALTER would be scanning under the strong lock it was written to avoid.
--
-- Bounded like every other lock this tree takes on a table it did not create:
-- VALIDATE's SHARE UPDATE EXCLUSIVE still conflicts with another one, so an
-- open transaction holding one would otherwise stall writers for as long as
-- this is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_owed_verdict_ruleset_shape;
