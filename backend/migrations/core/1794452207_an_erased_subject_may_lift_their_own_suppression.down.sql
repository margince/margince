SET LOCAL lock_timeout = '3s';

ALTER TABLE erasure_suppression DROP CONSTRAINT erasure_suppression_lift_is_whole;
ALTER TABLE erasure_suppression DROP COLUMN lift_contact_id;
ALTER TABLE erasure_suppression DROP COLUMN lift_consent_event_id;
ALTER TABLE erasure_suppression DROP COLUMN lifted_at;
