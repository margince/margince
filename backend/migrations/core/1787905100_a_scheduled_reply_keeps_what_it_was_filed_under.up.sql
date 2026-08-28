-- The records a scheduled REPLY was told to file itself under, beyond the ones
-- it inherits from its anchor.
--
-- A separate column rather than a second meaning for origin_links, which on an
-- account row is the WHOLE set. One column answering two questions depending on
-- origin_kind is a column a reader has to know the kind to interpret, and the
-- origin_shape check exists precisely so each kind's columns say what they are.
--
-- Null on an account origin, and on a reply that named nothing: a reply with no
-- additions is every reply sent before this column existed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE scheduled_send
    ADD COLUMN also_links jsonb;

-- ADD ... NOT VALID then VALIDATE, rather than one validated ADD.
--
-- Every existing row has NULL in a column added a statement ago, so the scan can
-- only pass — but Postgres still takes ACCESS EXCLUSIVE for its whole length,
-- and scheduled_send is read and written by the timer that releases messages.
-- NOT VALID takes that lock without scanning; VALIDATE downgrades to SHARE
-- UPDATE EXCLUSIVE for the pass. So a deployment blocks scheduling for a moment
-- rather than for a table scan.
--
-- The runner wraps each file in one transaction (dbmigrate.go), so a failure at
-- either statement rolls both back: the column can never be left without its
-- shape.
ALTER TABLE scheduled_send
    ADD CONSTRAINT scheduled_send_also_links_shape
    CHECK (also_links IS NULL
           OR (origin_kind = 'reply'::text AND jsonb_typeof(also_links) = 'array'::text)) NOT VALID;

ALTER TABLE scheduled_send VALIDATE CONSTRAINT scheduled_send_also_links_shape;
