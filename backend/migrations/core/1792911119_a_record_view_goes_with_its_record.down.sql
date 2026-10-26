SET LOCAL lock_timeout = '3s';

DROP TRIGGER trg_user_record_view_entity_keys ON user_record_view;
DROP FUNCTION trg_user_record_view_entity_keys();
ALTER TABLE user_record_view
	DROP COLUMN entity_company_id,
	DROP COLUMN entity_contact_id,
	DROP COLUMN entity_unkeyed;
