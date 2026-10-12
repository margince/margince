-- pgmigrate:no-transaction
-- The indexes a cascade from the new subject keys finds its rows by. Built
-- concurrently, because the table collects history and a plain build blocks
-- its writes for the length of a scan. Every row already there
-- has NULL keys, so each index starts empty. Each statement is safe to run
-- again: the drop ahead of a build clears an invalid index a failed build left
-- behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_deal;
CREATE INDEX CONCURRENTLY idx_assurance_task_item_subject_deal ON assurance_task_item (subject_deal_id) WHERE subject_deal_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_signal;
CREATE INDEX CONCURRENTLY idx_assurance_task_item_subject_signal ON assurance_task_item (subject_signal_id) WHERE subject_signal_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_offer;
CREATE INDEX CONCURRENTLY idx_assurance_task_item_subject_offer ON assurance_task_item (subject_offer_id) WHERE subject_offer_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_contract;
CREATE INDEX CONCURRENTLY idx_assurance_task_item_subject_contract ON assurance_task_item (subject_contract_id) WHERE subject_contract_id IS NOT NULL;
