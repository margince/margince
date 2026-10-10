-- The validation of the key and shape constraints the task item migration
-- added. A file of its own because VALIDATE takes a lighter
-- lock than the ADD, and only a separate transaction lets writers through
-- while it scans. None can fail: a row written before the keys reads
-- subject_unkeyed = true, which every CHECK admits, and has NULL keys, which every
-- foreign key admits.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_deal_id_fkey;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_signal_id_fkey;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_offer_id_fkey;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_contract_id_fkey;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_deal_id_bound;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_signal_id_bound;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_offer_id_bound;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_contract_id_bound;
ALTER TABLE assurance_task_item VALIDATE CONSTRAINT assurance_task_item_subject_shape;
