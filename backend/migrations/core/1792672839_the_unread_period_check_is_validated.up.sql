SET LOCAL lock_timeout = '3s';

-- Its own file, because VALIDATE scans the table and the ADD above must not
-- hold a lock for that scan.
ALTER TABLE capture_connection VALIDATE CONSTRAINT capture_connection_unread_period_whole;
