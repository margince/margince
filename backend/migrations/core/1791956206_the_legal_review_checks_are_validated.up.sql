-- The three checks widened beside this file are validated here, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log VALIDATE CONSTRAINT audit_log_action_check;
ALTER TABLE stage_exit_criterion VALIDATE CONSTRAINT stage_exit_criterion_kind_check;
ALTER TABLE deal_stage_evidence VALIDATE CONSTRAINT deal_stage_evidence_source_type_check;
