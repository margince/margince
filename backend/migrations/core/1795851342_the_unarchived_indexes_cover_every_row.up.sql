-- pgmigrate:no-transaction
-- This index leaves out archived rows, and nothing archives a sales
-- initiative. It is built again over every row, so the next migration can
-- drop the mark.
SET lock_timeout = '3s';

DROP INDEX CONCURRENTLY IF EXISTS sales_initiative_status_idx;
CREATE INDEX CONCURRENTLY sales_initiative_status_idx ON sales_initiative (status);

RESET lock_timeout;
