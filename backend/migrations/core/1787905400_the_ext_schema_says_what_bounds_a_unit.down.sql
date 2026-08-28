-- Partial by necessity: it restores the comment, and it cannot restore the two
-- dropped tables or their migration ledgers. Their contents belong to units this
-- tree no longer carries, so there is no source to replay them from — a down
-- that recreated empty tables would be worse, because a later reader would find
-- the shape and conclude the rows had been kept.
SET LOCAL lock_timeout = '3s';

COMMENT ON SCHEMA ext IS 'Extension tables (ADR-0069): ext_<name>_<table>, applied by the migrate role from each enabled unit''s own migrations. Tenant isolation is FORCE RLS plus a workspace-bound policy per table, NOT ownership — a per-unit ext_<name> owner role exists only in the pre-merge migration gate (issue #628). The core owns public; nothing here is core data.';
