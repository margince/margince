-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_waiting_address_time;
CREATE INDEX CONCURRENTLY idx_aparticipant_waiting_address_time
 ON activity_participant (lower(address), domain_evidence_at DESC)
 WHERE role = 'to' AND address IS NOT NULL;
