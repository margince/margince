-- pgmigrate:no-transaction
-- Erasure, retention, the corpus clear and the installation reset delete the
-- parents of these keys, and Postgres looks up each deleted row's children.
-- Without an index that is a scan of the child table per deleted row.
--
-- Built concurrently, so writes are never blocked for the build. The drop ahead
-- of each build clears an invalid index a failed build left behind. Partial
-- where most rows carry no reference.
DROP INDEX CONCURRENTLY IF EXISTS attachment_supersedes_ix;
CREATE INDEX CONCURRENTLY attachment_supersedes_ix ON attachment (supersedes_id) WHERE supersedes_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS deal_room_document_attachment_ix;
CREATE INDEX CONCURRENTLY deal_room_document_attachment_ix ON deal_room_document (attachment_id);
DROP INDEX CONCURRENTLY IF EXISTS deal_room_thread_attachment_ix;
CREATE INDEX CONCURRENTLY deal_room_thread_attachment_ix ON deal_room_thread (attachment_id) WHERE attachment_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS contact_phone_superseded_phone_ix;
CREATE INDEX CONCURRENTLY contact_phone_superseded_phone_ix ON contact_phone (superseded_phone_id) WHERE superseded_phone_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS preference_token_contact_email_ix;
CREATE INDEX CONCURRENTLY preference_token_contact_email_ix ON preference_token (contact_email_id) WHERE contact_email_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS comms_outbound_link_ix;
CREATE INDEX CONCURRENTLY comms_outbound_link_ix ON comms_outbound (link_id) WHERE link_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS communication_suppression_carried_from_ix;
CREATE INDEX CONCURRENTLY communication_suppression_carried_from_ix ON communication_suppression (carried_from) WHERE carried_from IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS voice_profile_delta_from_version_ix;
CREATE INDEX CONCURRENTLY voice_profile_delta_from_version_ix ON voice_profile_delta (voice_profile_id, from_version);
DROP INDEX CONCURRENTLY IF EXISTS voice_profile_version_predecessor_ix;
CREATE INDEX CONCURRENTLY voice_profile_version_predecessor_ix ON voice_profile_version (voice_profile_id, predecessor_version);
-- These two replace the voice_profile_id indexes, which become their prefixes.
-- The narrow ones are dropped only after the wide ones exist, so the profile
-- cascade always has an index.
DROP INDEX CONCURRENTLY IF EXISTS voice_build_profile_version_ix;
CREATE INDEX CONCURRENTLY voice_build_profile_version_ix ON voice_build (voice_profile_id, result_version);
DROP INDEX CONCURRENTLY IF EXISTS voice_learning_signal_profile_version_ix;
CREATE INDEX CONCURRENTLY voice_learning_signal_profile_version_ix ON voice_learning_signal (voice_profile_id, profile_version);
DROP INDEX CONCURRENTLY IF EXISTS voice_build_profile_fk;
DROP INDEX CONCURRENTLY IF EXISTS voice_learning_signal_profile_fk;
