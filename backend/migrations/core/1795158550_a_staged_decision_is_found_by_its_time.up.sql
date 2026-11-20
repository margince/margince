-- pgmigrate:no-transaction
-- The compliance count of sends per declared purpose reads the staging
-- decisions of a recent window. Without this index it walks every decision.
--
-- Built concurrently, so a send is never blocked for the build. The drop ahead
-- of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS communication_decision_staged_at;
CREATE INDEX CONCURRENTLY communication_decision_staged_at ON communication_decision (decided_at) WHERE phase = 'staging';
