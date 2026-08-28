-- Partial by necessity: it restores the comment, and it cannot restore the two
-- dropped tables or their migration ledgers. Their contents belong to units this
-- tree no longer carries, so there is no source to replay them from — a down
-- that recreated empty tables would be worse, because a later reader would find
-- the shape and conclude the rows had been kept.
SET LOCAL lock_timeout = '3s';

COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. A unit is bounded by the tables its migrations create and the grants they carry — there is no tenant column and no policy here, because an installation holds one workspace (ADR-0061) and no schema in this database carries either. A per-unit owner role would make the bound a grant rather than a convention; it exists today only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';
