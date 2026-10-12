SET LOCAL lock_timeout = '3s';

ALTER TABLE record_grant
	DROP COLUMN contact_id,
	DROP COLUMN company_id,
	DROP COLUMN deal_id,
	DROP COLUMN lead_id,
	DROP COLUMN project_id;
