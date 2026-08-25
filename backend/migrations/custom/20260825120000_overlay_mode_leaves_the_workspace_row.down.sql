-- Restore x_sor_mode and x_incumbent on the workspace row, and the three CHECKs
-- that governed them, exactly as the overlay baseline declared them.
--
-- The columns come back with their defaults first so the row satisfies
-- x_overlay_iff_incumbent at every instant: 'native' with a NULL incumbent is
-- the one pair that holds before any backfill. The constraints are added after
-- the values are restored, for the same reason.
SET LOCAL lock_timeout = '3s';

ALTER TABLE workspace
    ADD COLUMN x_sor_mode  text NOT NULL DEFAULT 'native',
    ADD COLUMN x_incumbent text;

-- The mode returns to the workspace the up half took it from. Every other
-- workspace row — an archived predecessor, say — keeps the default, which is
-- what it would have carried anyway: the up half read only the live one.
UPDATE workspace w
   SET x_sor_mode  = m.sor_mode,
       x_incumbent = m.incumbent
  FROM overlay_mode m
 WHERE w.id = (SELECT id FROM workspace WHERE archived_at IS NULL ORDER BY created_at, id LIMIT 1);

ALTER TABLE workspace
    ADD CONSTRAINT workspace_x_sor_mode_check CHECK (x_sor_mode IN ('native', 'overlay')),
    ADD CONSTRAINT workspace_x_incumbent_check
        CHECK (x_incumbent IS NULL OR x_incumbent IN ('hubspot', 'salesforce', 'dynamics')),
    ADD CONSTRAINT x_overlay_iff_incumbent
        CHECK ((x_sor_mode = 'overlay') = (x_incumbent IS NOT NULL));

DROP TABLE overlay_mode;
