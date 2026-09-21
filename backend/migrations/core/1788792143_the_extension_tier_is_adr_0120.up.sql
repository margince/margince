-- Two schema comments name ADR-0069 for the extension tier. That number is the
-- embedding-width decision; the extension tier is ADR-0120. The draft reused it
-- on an unmerged branch and the number reached the tree before anybody noticed.
--
-- A COMMENT is the one thing in a schema a reader cannot verify by reading the
-- objects around it: it either resolves to the right decision or it silently
-- sends them to another one. Somebody asking what bounds an extension's tables,
-- or what an extension secret handle is, landed on a decision about vector
-- dimensions.
--
-- Re-stated here rather than edited where they were written, because both are
-- applied and an applied version never re-runs: editing one would give FRESH
-- installations the corrected text while every deployed database kept the old,
-- and nothing would report the difference — the schema catalog records
-- structure, not comment prose, so the divergence has no gate to fail. That is
-- the same reason 1787905400_the_ext_schema_says_what_bounds_a_unit exists, and
-- its own text is carried through here unchanged but for the number.
--
-- The applied files keep saying ADR-0069 in their own prose, including the
-- baseline's dependency-order note, and that is correct rather than a leftover.
-- A migration's CONTENT is fingerprinted — dbmigrate stamps a digest per applied
-- version so a reader can tell "applied version X" from "applied the migration
-- this binary calls X" — so editing one, even a `--` line that executes nothing,
-- changes its identity against every database that already ran it. What those
-- files say is what was written; what the SCHEMA says is corrected here.
SET LOCAL lock_timeout = '3s';

COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0120): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. A unit is bounded by the tables its migrations create and the grants they carry — there is no tenant column and no policy here, because an installation holds one workspace (ADR-0061) and no schema in this database carries either. A per-unit owner role would make the bound a grant rather than a convention; it exists today only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';

COMMENT ON COLUMN extension_secret.vault_ref IS 'An opaque keyvault handle (ADR-0120): safe to log, never the secret itself.';
