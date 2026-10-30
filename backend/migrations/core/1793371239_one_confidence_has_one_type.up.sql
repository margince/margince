-- A confidence is numeric(4,3) everywhere, bounded 0 to 1.
--
-- Fifteen columns hold a machine's confidence in a derived fact and six types
-- held them. Three decimal places of exact decimal is what a confidence needs:
-- these values are compared against thresholds and shown on screen, and a
-- score that reads back as 0.8499999 when it was written as 0.85 produces a
-- report nobody can reproduce.
--
-- real and double precision cannot hold 0.85 exactly. numeric with no
-- precision accepts any scale, so two writers of one column can disagree.
--
-- capture_thread_verdict and ai_call had no range at all; theirs are added
-- NOT VALID here and validated in the file after this one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_request_settlement ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE capture_pending_counterparty ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE capture_thread_verdict ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE company_fact ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE company_profile_field ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE contact_provider_claim ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE deal_stage_evidence ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE field_provenance ALTER COLUMN confidence TYPE numeric(4,3);
ALTER TABLE signal ALTER COLUMN resolution_confidence TYPE numeric(4,3);
ALTER TABLE signal_resolution ALTER COLUMN match_confidence TYPE numeric(4,3);
ALTER TABLE ai_call ALTER COLUMN decision_confidence TYPE numeric(4,3);

ALTER TABLE capture_thread_verdict
  ADD CONSTRAINT capture_thread_verdict_confidence_range
  CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)) NOT VALID;

ALTER TABLE ai_call
  ADD CONSTRAINT ai_call_decision_confidence_range
  CHECK (decision_confidence IS NULL OR (decision_confidence >= 0 AND decision_confidence <= 1)) NOT VALID;
