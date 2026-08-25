-- Restore x_sor_mode and x_incumbent on the workspace row, and the three CHECKs
-- that governed them, exactly as the overlay baseline declared them.
--
-- The constraints are added AFTER the values are restored, so no intermediate
-- state is ever checked: the columns arrive at 'native' with a NULL incumbent,
-- the backfill moves them together, and only then does
-- x_overlay_iff_incumbent start judging the pair.
SET LOCAL lock_timeout = '3s';

-- Both tables, up front and for the whole migration, for the reason the up half
-- gives in mirror: this reads overlay_mode and only later DROPs it, and a write
-- committed into that window would be copied nowhere and then destroyed.
LOCK TABLE workspace, overlay_mode IN ACCESS EXCLUSIVE MODE;

ALTER TABLE workspace
    ADD COLUMN x_sor_mode  text NOT NULL DEFAULT 'native',
    ADD COLUMN x_incumbent text;

-- The mode returns to the workspace the up half took it from. Every other
-- workspace row — an archived predecessor, say — comes back at the default
-- rather than at whatever it held before: the up half read only the live one,
-- so an archived workspace's mode does not survive up-then-down. Nothing reads
-- it, which is why that is acceptable and not merely unnoticed.
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
DROP FUNCTION overlay_mode_undeletable();
