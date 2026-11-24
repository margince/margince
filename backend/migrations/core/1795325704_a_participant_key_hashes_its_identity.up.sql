-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- The key compares one digest of the four identity columns, so no entry repeats the address text.
-- A null column adds nothing to the digest input and a text adds its length and itself.
-- So a null, an empty text and a text holding the separator each give a different input.
DROP INDEX CONCURRENTLY IF EXISTS uq_activity_participant_identity;
CREATE UNIQUE INDEX CONCURRENTLY uq_activity_participant_identity
 ON activity_participant (activity_id, role, (md5(
  coalesce(user_id::text, '') || '|' || coalesce(contact_id::text, '') || '|'
  || coalesce(length(address)::text || ':' || address, '') || '|'
  || coalesce(length(channel_user_id)::text || ':' || channel_user_id, ''))::uuid));
RESET lock_timeout;
