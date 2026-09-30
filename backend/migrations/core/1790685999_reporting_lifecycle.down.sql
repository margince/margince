SET LOCAL lock_timeout = '3s';
-- Operational rollback uses the feature flag; only an unused schema can be reversed.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM report_definition) OR EXISTS(SELECT 1 FROM sales_target) OR EXISTS(SELECT 1 FROM reporting_framework_revision) OR EXISTS(SELECT 1 FROM deal_stage_history WHERE pipeline_id_at_change IS NOT NULL) OR EXISTS(SELECT 1 FROM activity_meeting_history WHERE customer_eligible_at_change IS NOT NULL) OR EXISTS(SELECT 1 FROM sdr_handoff_event WHERE submitter_id_at_change IS NOT NULL) THEN
  RAISE EXCEPTION 'Reporting history exists; disable analytics.performance_enabled instead of reversing migrations';
 END IF;
END $$;
DROP TABLE report_edition_contribution,report_edition,report_execution,report_schedule,report_definition_revision,report_definition,sales_target_revision,sales_target,reporting_framework_revision,reporting_framework;
DROP INDEX reporting_stage_events;
DROP INDEX reporting_primary_credit;
ALTER TABLE deal_stage_history DROP COLUMN owner_id_at_change, DROP COLUMN pipeline_id_at_change,
 DROP COLUMN base_minor_at_change, DROP COLUMN base_currency_at_change, DROP COLUMN fx_rate_at_change,
 DROP COLUMN fx_date_at_change, DROP COLUMN valuation_provenance;
ALTER TABLE activity_meeting_history DROP COLUMN host_id_at_change, DROP COLUMN customer_eligible_at_change;
ALTER TABLE sdr_handoff_event DROP COLUMN deal_id_at_change, DROP COLUMN submitter_id_at_change;
