-- pgmigrate:no-transaction
SET lock_timeout = '3s';

-- Spent confirmations must remain reachable when their subject or confirmer is deleted.
DROP INDEX CONCURRENTLY IF EXISTS lawful_basis_confirmation_contact_id_cascade_idx;
CREATE INDEX CONCURRENTLY lawful_basis_confirmation_contact_id_cascade_idx ON lawful_basis_confirmation (contact_id);
DROP INDEX CONCURRENTLY IF EXISTS lawful_basis_confirmation_lead_id_cascade_idx;
CREATE INDEX CONCURRENTLY lawful_basis_confirmation_lead_id_cascade_idx ON lawful_basis_confirmation (lead_id);
DROP INDEX CONCURRENTLY IF EXISTS lawful_basis_confirmation_confirmed_by_cascade_idx;
CREATE INDEX CONCURRENTLY lawful_basis_confirmation_confirmed_by_cascade_idx ON lawful_basis_confirmation (confirmed_by);

DROP INDEX CONCURRENTLY IF EXISTS confirm_token_subject_request_id_idx;
CREATE INDEX CONCURRENTLY confirm_token_subject_request_id_idx ON confirm_token (subject_request_id);
DROP INDEX CONCURRENTLY IF EXISTS consent_event_subject_request_id_idx;
CREATE INDEX CONCURRENTLY consent_event_subject_request_id_idx ON consent_event (subject_request_id);

RESET lock_timeout;
