-- A suggestion's cited evidence outlives the record it cites: deleting an activity,
-- signal or attachment leaves the evidence row with the cited id unnamed. It used
-- to take the evidence away and leave a suggestion without its proof. The shape
-- check still names one kind per row but admits the cited id being gone. Added
-- NOT VALID and validated in the file beside this one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_suggestion_evidence
    DROP CONSTRAINT deal_suggestion_evidence_shape,
    ADD CONSTRAINT deal_suggestion_evidence_shape CHECK (
        (kind = 'meeting' AND signal_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'signal' AND activity_id IS NULL AND attachment_id IS NULL)
     OR (kind = 'attachment' AND activity_id IS NULL AND signal_id IS NULL)) NOT VALID,
    DROP CONSTRAINT deal_suggestion_evidence_activity_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_activity_id_fkey FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE SET NULL NOT VALID,
    DROP CONSTRAINT deal_suggestion_evidence_attachment_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_attachment_id_fkey FOREIGN KEY (attachment_id) REFERENCES attachment(id) ON DELETE SET NULL NOT VALID,
    DROP CONSTRAINT deal_suggestion_evidence_signal_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_signal_id_fkey FOREIGN KEY (signal_id) REFERENCES signal(id) ON DELETE SET NULL NOT VALID;
