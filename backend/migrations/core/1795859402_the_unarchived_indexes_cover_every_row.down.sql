-- pgmigrate:no-transaction
SET lock_timeout = '3s';

DROP INDEX CONCURRENTLY IF EXISTS sales_initiative_status_idx;
CREATE INDEX CONCURRENTLY sales_initiative_status_idx ON sales_initiative (status) WHERE archived_at IS NULL;

RESET lock_timeout;
