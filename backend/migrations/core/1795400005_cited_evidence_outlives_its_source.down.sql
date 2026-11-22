-- Restores the cascade and the shape check that names the cited id. A row whose
-- cited record was deleted holds NULL there, so the check refuses it until that
-- evidence is dealt with.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_suggestion_evidence
    DROP CONSTRAINT deal_suggestion_evidence_shape,
    ADD CONSTRAINT deal_suggestion_evidence_shape CHECK (
        (kind = 'meeting' AND activity_id IS NOT NULL AND signal_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'signal' AND signal_id IS NOT NULL AND activity_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'attachment' AND attachment_id IS NOT NULL AND activity_id IS NULL AND signal_id IS NULL)),
    DROP CONSTRAINT deal_suggestion_evidence_activity_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_activity_id_fkey FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    DROP CONSTRAINT deal_suggestion_evidence_attachment_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_attachment_id_fkey FOREIGN KEY (attachment_id) REFERENCES attachment(id) ON DELETE CASCADE,
    DROP CONSTRAINT deal_suggestion_evidence_signal_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_signal_id_fkey FOREIGN KEY (signal_id) REFERENCES signal(id) ON DELETE CASCADE;
