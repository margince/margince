-- pgmigrate:no-transaction
-- Restores the raw-host index, concurrently and safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_sender_host_reversed;
CREATE INDEX CONCURRENTLY idx_aparticipant_sender_host_reversed ON activity_participant
    ((reverse(split_part(lower(address), '@', 2)) COLLATE "C"))
    WHERE role = 'from' AND address IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_sender_domain_reversed;
