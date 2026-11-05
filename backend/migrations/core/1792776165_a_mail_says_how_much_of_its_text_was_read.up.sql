-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- How much of a captured mail's text the parser decoded: all of it, part of it
-- (a text part was not read in full), none (the body holds only the From and To
-- lines), or only a forwarded original's (not the sender's own words). Rows that exist now are 'unassessed': nothing has checked them, and
-- the data task core/1792776165_mail_body_completeness assesses captured mail
-- from its stored original. A constant default adds the column with no
-- rewrite, and every row written from now on starts as complete.
ALTER TABLE activity
    ADD COLUMN body_completeness text NOT NULL DEFAULT 'unassessed';
ALTER TABLE activity
    ALTER COLUMN body_completeness SET DEFAULT 'complete';

-- NOT VALID: the table holds history, and the scan runs in the next migration
-- under a lock readers and writers pass. Every existing row is 'unassessed'.
ALTER TABLE activity
    ADD CONSTRAINT activity_body_completeness_check
        CHECK (body_completeness IN ('complete', 'partial', 'unreadable', 'forwarded', 'unassessed')) NOT VALID;
