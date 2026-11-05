-- The validation of the key and shape constraints the signal, weekly
-- commitment and record view migrations added. A file of its own because
-- VALIDATE takes a lighter lock than the ADD, and only a separate transaction
-- lets writers through while it scans. None can fail: a row written before
-- the keys reads its marker as true, which every CHECK admits, and has NULL
-- keys, which every foreign key admits.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_deal_id_fkey;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_company_id_fkey;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_contact_id_fkey;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_project_id_fkey;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_deal_id_bound;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_company_id_bound;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_contact_id_bound;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_project_id_bound;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_shape;

ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_deal_id_fkey;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_lead_id_fkey;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_contact_id_fkey;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_company_id_fkey;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_project_id_fkey;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_deal_id_bound;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_lead_id_bound;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_contact_id_bound;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_company_id_bound;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_project_id_bound;
ALTER TABLE weekly_plan_commitment VALIDATE CONSTRAINT weekly_plan_commitment_linked_record_shape;

ALTER TABLE user_record_view VALIDATE CONSTRAINT user_record_view_entity_company_id_fkey;
ALTER TABLE user_record_view VALIDATE CONSTRAINT user_record_view_entity_contact_id_fkey;
ALTER TABLE user_record_view VALIDATE CONSTRAINT user_record_view_entity_company_id_bound;
ALTER TABLE user_record_view VALIDATE CONSTRAINT user_record_view_entity_contact_id_bound;
ALTER TABLE user_record_view VALIDATE CONSTRAINT user_record_view_entity_shape;
