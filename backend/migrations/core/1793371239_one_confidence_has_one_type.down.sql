-- Back to the six types, each column as this migration found it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call DROP CONSTRAINT IF EXISTS ai_call_decision_confidence_range;
ALTER TABLE capture_thread_verdict DROP CONSTRAINT IF EXISTS capture_thread_verdict_confidence_range;

ALTER TABLE activity_request_settlement ALTER COLUMN confidence TYPE numeric;
ALTER TABLE capture_pending_counterparty ALTER COLUMN confidence TYPE numeric;
ALTER TABLE capture_thread_verdict ALTER COLUMN confidence TYPE numeric;
ALTER TABLE company_fact ALTER COLUMN confidence TYPE real;
ALTER TABLE company_profile_field ALTER COLUMN confidence TYPE real;
ALTER TABLE contact_provider_claim ALTER COLUMN confidence TYPE numeric(5,4);
ALTER TABLE deal_stage_evidence ALTER COLUMN confidence TYPE numeric(3,2);
ALTER TABLE field_provenance ALTER COLUMN confidence TYPE real;
ALTER TABLE signal ALTER COLUMN resolution_confidence TYPE numeric;
ALTER TABLE signal_resolution ALTER COLUMN match_confidence TYPE numeric;
ALTER TABLE ai_call ALTER COLUMN decision_confidence TYPE double precision;
