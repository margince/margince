-- A suggestion's cited evidence outlives the record it cites: deleting an activity,
-- signal or attachment leaves the evidence row with the cited id cleared and
-- source_gone set, where it used to take the evidence away. The shape check tells
-- a cited-and-gone row from one that never cited anything. Added NOT VALID and
-- validated in the file beside this one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_suggestion_evidence ADD COLUMN source_gone boolean NOT NULL DEFAULT false;

-- source_gone is set by the database when a cited record is deleted, never by a
-- writer: an insert always starts false, and an update turns it on only when a
-- cited id was cleared by the foreign key's SET NULL.
CREATE FUNCTION deal_suggestion_evidence_keeps_its_source_gone() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    NEW.source_gone := false;
    RETURN NEW;
  END IF;
  NEW.source_gone := OLD.source_gone
    OR (OLD.activity_id IS NOT NULL AND NEW.activity_id IS NULL)
    OR (OLD.signal_id IS NOT NULL AND NEW.signal_id IS NULL)
    OR (OLD.attachment_id IS NOT NULL AND NEW.attachment_id IS NULL);
  RETURN NEW;
END;
$$;

CREATE TRIGGER deal_suggestion_evidence_keeps_source_gone
    BEFORE INSERT OR UPDATE ON deal_suggestion_evidence
    FOR EACH ROW EXECUTE FUNCTION deal_suggestion_evidence_keeps_its_source_gone();

ALTER TABLE deal_suggestion_evidence
    DROP CONSTRAINT deal_suggestion_evidence_shape,
    ADD CONSTRAINT deal_suggestion_evidence_shape CHECK (
        (kind = 'meeting' AND signal_id IS NULL AND attachment_id IS NULL AND (activity_id IS NOT NULL OR source_gone))
     OR (kind = 'signal' AND activity_id IS NULL AND attachment_id IS NULL AND (signal_id IS NOT NULL OR source_gone))
     OR (kind = 'attachment' AND activity_id IS NULL AND signal_id IS NULL AND (attachment_id IS NOT NULL OR source_gone))) NOT VALID,
    DROP CONSTRAINT deal_suggestion_evidence_activity_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_activity_id_fkey FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE SET NULL NOT VALID,
    DROP CONSTRAINT deal_suggestion_evidence_attachment_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_attachment_id_fkey FOREIGN KEY (attachment_id) REFERENCES attachment(id) ON DELETE SET NULL NOT VALID,
    DROP CONSTRAINT deal_suggestion_evidence_signal_id_fkey,
    ADD CONSTRAINT deal_suggestion_evidence_signal_id_fkey FOREIGN KEY (signal_id) REFERENCES signal(id) ON DELETE SET NULL NOT VALID;
