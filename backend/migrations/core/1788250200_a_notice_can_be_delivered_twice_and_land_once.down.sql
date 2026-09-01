SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_notice_dedupe;
ALTER TABLE notice DROP COLUMN dedupe_key;
