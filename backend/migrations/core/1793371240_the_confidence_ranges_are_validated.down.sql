-- Put both ranges back present and unvalidated, as this migration found them.
-- Postgres has no ALTER that un-validates, so each is dropped and re-added in
-- one statement, leaving no window with the rule absent.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_thread_verdict
    DROP CONSTRAINT capture_thread_verdict_confidence_range,
    ADD CONSTRAINT capture_thread_verdict_confidence_range
        CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)) NOT VALID;

ALTER TABLE ai_call
    DROP CONSTRAINT ai_call_decision_confidence_range,
    ADD CONSTRAINT ai_call_decision_confidence_range
        CHECK (decision_confidence IS NULL OR (decision_confidence >= 0 AND decision_confidence <= 1)) NOT VALID;
