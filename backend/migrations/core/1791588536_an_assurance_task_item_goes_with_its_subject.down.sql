SET LOCAL lock_timeout = '3s';

DROP TRIGGER trg_assurance_task_item_subject_keys ON assurance_task_item;
DROP FUNCTION trg_assurance_task_item_subject_keys();
ALTER TABLE assurance_task_item
	DROP COLUMN subject_deal_id,
	DROP COLUMN subject_signal_id,
	DROP COLUMN subject_offer_id,
	DROP COLUMN subject_contract_id,
	DROP COLUMN subject_unkeyed;
