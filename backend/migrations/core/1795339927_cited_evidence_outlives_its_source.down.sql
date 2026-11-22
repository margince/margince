-- Restores the cascade and the strict shape.
SET LOCAL lock_timeout = '5s';

-- The strict shape comes back NOT VALID: a row whose cited record was deleted holds
-- NULL there, and the new rule still holds for every row written after this one.
ALTER TABLE deal_suggestion_evidence
    DROP CONSTRAINT deal_suggestion_evidence_shape,
    ADD CONSTRAINT deal_suggestion_evidence_shape CHECK (
        (kind = 'meeting' AND activity_id IS NOT NULL AND signal_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'signal' AND signal_id IS NOT NULL AND activity_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'attachment' AND attachment_id IS NOT NULL AND activity_id IS NULL AND signal_id IS NULL)) NOT VALID;
DROP TRIGGER deal_suggestion_evidence_keeps_source_gone ON deal_suggestion_evidence;
DROP FUNCTION deal_suggestion_evidence_keeps_its_source_gone();
ALTER TABLE deal_suggestion_evidence DROP COLUMN source_gone;
ALTER TABLE deal_suggestion_evidence
    DROP CONSTRAINT deal_suggestion_evidence_activity_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_activity_id_fkey FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    DROP CONSTRAINT deal_suggestion_evidence_attachment_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_attachment_id_fkey FOREIGN KEY (attachment_id) REFERENCES attachment(id) ON DELETE CASCADE,
    DROP CONSTRAINT deal_suggestion_evidence_signal_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_signal_id_fkey FOREIGN KEY (signal_id) REFERENCES signal(id) ON DELETE CASCADE;
