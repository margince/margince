-- Rolls the data back to what the binary before legal review can read, then
-- narrows the two checks it widened. The file beside this one validates them.
--
-- A rollback DISCARDS every legal_review evidence row, so no stage reads as
-- legal-approved. It CONVERTS every legal_reviewed criterion to custom: the
-- stage keeps the line, and the line stops blocking a move. The reviews
-- themselves go with their table in the file beside this one.
--
-- The audit vocabulary is NOT narrowed. audit_log is append-only, so a request
-- or withdrawal already recorded cannot be removed, and the older binary reads
-- an unknown verb as plain text.
SET LOCAL lock_timeout = '5s';

DELETE FROM deal_stage_evidence WHERE source_type = 'legal_review';

UPDATE stage_exit_criterion SET kind = 'custom' WHERE kind = 'legal_reviewed';

ALTER TABLE deal_stage_evidence DROP CONSTRAINT deal_stage_evidence_source_type_check;
ALTER TABLE deal_stage_evidence ADD CONSTRAINT deal_stage_evidence_source_type_check
    CHECK (source_type IN ('activity', 'contract')) NOT VALID;

ALTER TABLE stage_exit_criterion DROP CONSTRAINT stage_exit_criterion_kind_check;
ALTER TABLE stage_exit_criterion ADD CONSTRAINT stage_exit_criterion_kind_check
    CHECK (kind IN ('buyer_confirmed', 'event_held', 'document_signed',
                    'role_identified', 'terms_accepted', 'custom')) NOT VALID;
