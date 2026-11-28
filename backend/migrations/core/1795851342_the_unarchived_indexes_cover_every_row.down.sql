-- pgmigrate:no-transaction
SET lock_timeout = '3s';

DROP INDEX CONCURRENTLY IF EXISTS voice_build_deferred_due;
CREATE INDEX CONCURRENTLY voice_build_deferred_due ON voice_build (next_attempt_at, id) WHERE status = 'deferred' AND archived_at IS NULL;

DROP INDEX CONCURRENTLY IF EXISTS voice_build_poll;
CREATE INDEX CONCURRENTLY voice_build_poll ON voice_build (voice_profile_id, created_at DESC, id DESC) WHERE archived_at IS NULL;

DROP INDEX CONCURRENTLY IF EXISTS voice_profile_delta_history;
CREATE INDEX CONCURRENTLY voice_profile_delta_history ON voice_profile_delta (voice_profile_id, created_at DESC, id DESC) WHERE archived_at IS NULL;

DROP INDEX CONCURRENTLY IF EXISTS voice_profile_version_history;
CREATE INDEX CONCURRENTLY voice_profile_version_history ON voice_profile_version (voice_profile_id, created_at DESC, id DESC) WHERE archived_at IS NULL;

DROP INDEX CONCURRENTLY IF EXISTS sales_initiative_status_idx;
CREATE INDEX CONCURRENTLY sales_initiative_status_idx ON sales_initiative (status) WHERE archived_at IS NULL;

RESET lock_timeout;
