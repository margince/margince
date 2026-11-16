-- pgmigrate:no-transaction
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS lawful_basis_confirmation_confirmed_by_cascade_idx;
DROP INDEX CONCURRENTLY IF EXISTS lawful_basis_confirmation_lead_id_cascade_idx;
DROP INDEX CONCURRENTLY IF EXISTS lawful_basis_confirmation_contact_id_cascade_idx;
DROP INDEX CONCURRENTLY IF EXISTS confirm_token_subject_request_id_idx;
DROP INDEX CONCURRENTLY IF EXISTS consent_event_subject_request_id_idx;

RESET lock_timeout;
