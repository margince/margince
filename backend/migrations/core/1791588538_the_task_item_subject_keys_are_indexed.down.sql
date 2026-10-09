-- Dropped concurrently, each step safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_deal;
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_signal;
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_offer;
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_task_item_subject_contract;
