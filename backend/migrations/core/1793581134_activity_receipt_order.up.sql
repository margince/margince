SET LOCAL lock_timeout = '3s';
CREATE INDEX idx_activity_receipt_order ON activity (created_at, id) WHERE archived_at IS NULL;
