SET LOCAL lock_timeout = '3s';

ALTER INDEX idx_import_run_status RENAME TO idx_import_run_ws;
