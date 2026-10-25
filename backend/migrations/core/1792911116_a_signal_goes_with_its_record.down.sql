SET LOCAL lock_timeout = '3s';

DROP TRIGGER trg_signal_entity_keys ON signal;
DROP FUNCTION trg_signal_entity_keys();
ALTER TABLE signal
	DROP COLUMN entity_deal_id,
	DROP COLUMN entity_company_id,
	DROP COLUMN entity_contact_id,
	DROP COLUMN entity_project_id,
	DROP COLUMN entity_unkeyed;
