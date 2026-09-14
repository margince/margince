SET LOCAL lock_timeout = '3s';
ALTER TABLE capture_backfill DROP CONSTRAINT capture_backfill_window_months_check;
ALTER TABLE capture_backfill ADD CONSTRAINT capture_backfill_window_months_check
    CHECK (window_months IN (3, 6, 12, 24, 36, 60, 84, 120));
