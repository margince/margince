-- Back to the schema without legal review. The file after this one has
-- already removed the evidence and converted the criteria (it says what that
-- discards); this validates the two checks it narrowed and drops the reviews.
--
-- The audit check keeps the two legal review verbs, for the reason that file
-- gives, and the role grants stay, for the reason 1790516373's down gives.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_stage_evidence VALIDATE CONSTRAINT deal_stage_evidence_source_type_check;
ALTER TABLE stage_exit_criterion VALIDATE CONSTRAINT stage_exit_criterion_kind_check;

DROP TABLE IF EXISTS legal_review;
