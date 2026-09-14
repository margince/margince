-- Validate the purpose_id foreign key added NOT VALID in the previous migration.
--
-- This is the second half of the split the sibling add-migration explains: its
-- transaction committed and released the ADD's lock, writers went through, and
-- this file now scans the table on its own. VALIDATE CONSTRAINT takes only
-- SHARE UPDATE EXCLUSIVE — it does not block INSERT/UPDATE/DELETE — but the
-- lock_timeout guard still bounds how long the brief acquisition may queue
-- behind another transaction on this table read on every send.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression
    VALIDATE CONSTRAINT communication_suppression_purpose_id_fkey;
