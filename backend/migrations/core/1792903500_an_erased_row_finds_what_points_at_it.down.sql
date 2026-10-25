-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS voice_build_profile_fk;
CREATE INDEX CONCURRENTLY voice_build_profile_fk ON voice_build (voice_profile_id);
DROP INDEX CONCURRENTLY IF EXISTS voice_learning_signal_profile_fk;
CREATE INDEX CONCURRENTLY voice_learning_signal_profile_fk ON voice_learning_signal (voice_profile_id);
DROP INDEX CONCURRENTLY IF EXISTS voice_learning_signal_profile_version_ix;
DROP INDEX CONCURRENTLY IF EXISTS voice_build_profile_version_ix;
DROP INDEX CONCURRENTLY IF EXISTS voice_profile_version_predecessor_ix;
DROP INDEX CONCURRENTLY IF EXISTS voice_profile_delta_from_version_ix;
DROP INDEX CONCURRENTLY IF EXISTS communication_suppression_carried_from_ix;
DROP INDEX CONCURRENTLY IF EXISTS comms_outbound_link_ix;
DROP INDEX CONCURRENTLY IF EXISTS preference_token_contact_email_ix;
DROP INDEX CONCURRENTLY IF EXISTS contact_phone_superseded_phone_ix;
DROP INDEX CONCURRENTLY IF EXISTS deal_room_thread_attachment_ix;
DROP INDEX CONCURRENTLY IF EXISTS deal_room_document_attachment_ix;
DROP INDEX CONCURRENTLY IF EXISTS attachment_supersedes_ix;
