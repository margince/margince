SET LOCAL lock_timeout = '3s';

ALTER TABLE signal_thread_scan
    DROP CONSTRAINT IF EXISTS signal_thread_scan_scanned_from_shape;

ALTER TABLE signal_thread_scan
    DROP COLUMN IF EXISTS scanned_from,
    DROP COLUMN IF EXISTS scanned_from_id;
