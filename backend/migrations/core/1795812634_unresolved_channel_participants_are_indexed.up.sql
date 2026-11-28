-- pgmigrate:no-transaction
-- The repair seeks unresolved parties by account instead of walking a contact's history.
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_unresolved_channel;
CREATE INDEX CONCURRENTLY idx_aparticipant_unresolved_channel
 ON activity_participant (channel_user_id, activity_id, id)
 WHERE channel_user_id IS NOT NULL AND contact_id IS NULL AND user_id IS NULL AND NOT past_party_cap;
