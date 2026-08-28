SET LOCAL lock_timeout = '3s';

ALTER TABLE signal_thread_scan
    DROP COLUMN IF EXISTS scanned_from;
