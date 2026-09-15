-- The incumbent-mirror cluster leaves the schema.
--
-- Twelve tables, one singleton mode row, one delete guard and one partial index
-- carried the HubSpot overlay: a mirrored read cache of a customer's incumbent
-- CRM, the connection that fed it, the per-user visibility projection it was
-- read through, and the flip that would have retired it. Nothing reads or
-- writes any of them any more.
--
-- Dropped rather than left standing. An unread table is not inert: it is a
-- schema a reader has to rule out, a set of grants the role separation keeps
-- issuing, and a row the reset sweep keeps clearing. The `system_log` index is
-- partial on an action nothing writes, so it indexes nothing and is read by
-- nobody.
--
-- CASCADE only where the table OWNS what would block it: overlay_mode carries
-- its own two triggers, and the app_user foreign keys point INTO these tables,
-- never out of them, so nothing outside this cluster loses a constraint here.

-- The DROPs below take ACCESS EXCLUSIVE on tables this migration did not
-- create, so the wait is bounded rather than left to queue behind whatever
-- open transaction happens to hold a conflicting lock.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_system_log_mirror_conflict_class;

DROP TRIGGER IF EXISTS trg_overlay_mode_undeletable ON overlay_mode;
DROP TRIGGER IF EXISTS trg_overlay_mode_updated ON overlay_mode;
DROP FUNCTION IF EXISTS overlay_mode_undeletable();

DROP TABLE IF EXISTS overlay_write_ledger;
DROP TABLE IF EXISTS overlay_tombstone;
DROP TABLE IF EXISTS overlay_sync_state;
DROP TABLE IF EXISTS overlay_reconcile_watermark;
DROP TABLE IF EXISTS overlay_mirror_halt;
DROP TABLE IF EXISTS overlay_mirror;
DROP TABLE IF EXISTS overlay_backfill_cursor;
DROP TABLE IF EXISTS overlay_association;
DROP TABLE IF EXISTS overlay_mode;
DROP TABLE IF EXISTS mirror_visibility;
DROP TABLE IF EXISTS mirror_user_map;
DROP TABLE IF EXISTS mirror_user_automap_block;
DROP TABLE IF EXISTS incumbent_connection;

-- The flip's two import connectors go with it: `mirror` read the frozen mirror
-- snapshot and `bundle` read a pre-flip export. Neither names a source anything
-- can produce now, and a run already carrying one is a run of a feature that no
-- longer exists, so the rows go before the CHECK narrows.
DELETE FROM import_record_map WHERE source_system IN ('mirror', 'bundle');
DELETE FROM import_run WHERE connector IN ('mirror', 'bundle');

ALTER TABLE import_run DROP CONSTRAINT IF EXISTS import_run_connector_check;
ALTER TABLE import_run ADD CONSTRAINT import_run_connector_check
    CHECK (connector IN ('csv', 'hubspot', 'salesforce'));
