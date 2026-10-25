SET LOCAL lock_timeout = '3s';

DROP TRIGGER trg_weekly_plan_commitment_linked_keys ON weekly_plan_commitment;
DROP FUNCTION trg_weekly_plan_commitment_linked_keys();
ALTER TABLE weekly_plan_commitment
	DROP COLUMN linked_deal_id,
	DROP COLUMN linked_lead_id,
	DROP COLUMN linked_contact_id,
	DROP COLUMN linked_company_id,
	DROP COLUMN linked_project_id,
	DROP COLUMN linked_record_unkeyed;
