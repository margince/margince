-- ADR-0091: retire workspace.slug.
--
-- The column was the tenant's subdomain label, from when a request chose its
-- workspace by hostname. One installation serves one organization (ADR-0061),
-- nothing resolves a subdomain, and no reader is left: bootstrap INSERTed it
-- and not one statement in the tree SELECTed it back. What still uses the
-- derived string uses it in memory, to build the agent seat's local address,
-- and keeps doing so without the column.
--
-- A write-only column is worse than an absent one. It reads as live to whoever
-- finds it next, and this one already cost two integration suites: they kept
-- writing a value that governed nothing, and collided on workspace_slug_unique
-- while doing it.
--
-- The UNIQUE constraint and its index belong to the column and go with it.
--
-- Bounded, because DROP COLUMN takes ACCESS EXCLUSIVE on a table this migration
-- did not create: an open transaction holding a conflicting lock would stall
-- every write to `workspace` for as long as this is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE workspace DROP COLUMN slug;
