-- A mailbox can be excluded from the nightly signature pass, on its own.
--
-- The workspace-level setting already exists in spirit (capture.auto_enrich);
-- what has been missing is the granularity a works-council negotiation actually
-- asks for: this mailbox, not this workspace. The exclusion list is a different
-- lever — it filters whole MESSAGES by address or domain, and says nothing
-- about whether the mail that IS captured may be read for a signature.
--
-- Nullable on purpose, and the null is the third state rather than a missing
-- value: it means "whatever the workspace says", so an admin flipping the
-- workspace default moves every mailbox that never made its own choice, and
-- leaves the ones that did. Collapsing that into a boolean with a default would
-- freeze today's workspace answer onto every existing row at migration time and
-- make the default un-followable afterwards.
-- Adding a column takes an ACCESS EXCLUSIVE lock on a table every connector
-- sync writes, so the wait for it is bounded rather than open-ended: an
-- unrelated transaction holding a conflicting lock would otherwise stall every
-- capture write for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection
    ADD COLUMN signature_enrich_enabled boolean;

COMMENT ON COLUMN capture_connection.signature_enrich_enabled IS
    'Per-mailbox switch for the nightly signature pass. NULL inherits the workspace setting (capture.signature_enrich); false means no mail from this mailbox is ever selected for enrichment.';
