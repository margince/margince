-- The ext schema's own comment described a tenant boundary the tier no longer
-- has, and two retired demo units left their tables behind.
--
-- A COMMENT is a schema fact, so correcting one is a migration and not an edit
-- to the baseline: an applied version never re-runs, so editing the baseline
-- would change what a fresh installation gets while every deployed database
-- kept the old text.
--
-- What bounds a unit now is its own migrations and the grants they carry, which
-- is what extmigrategate polices — and the same gate refuses a unit table that
-- declares workspace_id or a policy over it.
SET LOCAL lock_timeout = '3s';

COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. A unit is bounded by the tables its migrations create and the grants they carry — there is no tenant column and no policy here, because an installation holds one workspace (ADR-0061) and no schema in this database carries either. A per-unit owner role would make the bound a grant rather than a convention; it exists today only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';

-- The `notes` and `relay-probe` demo units are removed in the same change that
-- carries this migration. Nothing runs a removed unit's down-migrations and
-- compose/datasweep.go sweeps `public` only, so without this their tables and
-- migration ledgers would outlive them on every installation that ever enabled
-- them.
--
-- A core migration naming extension tables is an exception and not a pattern:
-- the tier has otherwise kept core ignorant of unit table names, and a unit that
-- is REMOVED rather than retired is the one case with no other owner. Leaving
-- them to the operator is worse in a tree people re-seed.
--
-- IF EXISTS on every statement, because this same migration runs on fresh
-- installations where none of these objects were ever created.
DROP TABLE IF EXISTS ext.ext_notes_note;
DROP TABLE IF EXISTS ext.ext_relay_probe_connection;

DROP TABLE IF EXISTS schema_migrations_ext_notes;
DROP TABLE IF EXISTS schema_migrations_ext_relay_probe;
