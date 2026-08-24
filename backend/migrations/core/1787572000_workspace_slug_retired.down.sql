-- Restore workspace.slug: NOT NULL and UNIQUE, as 0001 declared it.
--
-- Added nullable, then backfilled, then constrained — the column has no default
-- that could satisfy UNIQUE for more than one row, so a plain
-- `ADD COLUMN ... NOT NULL DEFAULT` would fail on the second. The id is the one
-- value every row already holds that is unique by construction, and a uuid's
-- text form is subdomain-safe, which is the shape the column was declared for.
--
-- The reversal restores the SCHEMA, not the slugs: the strings were derived
-- from the organization name at bootstrap and are not recoverable from a
-- database that no longer stores them. Anything that needs the derived form
-- computes it, which is what let the column go in the first place.
--
-- The backfill DOES touch data, in one way worth naming: trg_workspace_updated
-- fires on it, so every workspace row comes back with updated_at moved to the
-- rollback. That is accepted rather than suppressed — the row genuinely was
-- written, and a reversal that lied about when would be the worse artefact.
SET LOCAL lock_timeout = '3s';

ALTER TABLE workspace ADD COLUMN slug text;
UPDATE workspace SET slug = id::text;
ALTER TABLE workspace ALTER COLUMN slug SET NOT NULL;
ALTER TABLE workspace ADD CONSTRAINT workspace_slug_unique UNIQUE (slug);
