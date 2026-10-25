-- pgmigrate:no-transaction
-- Dropped concurrently, each step safe to run again.
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_deal;
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_company;
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_contact;
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_project;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_deal;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_lead;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_contact;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_company;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_project;
DROP INDEX CONCURRENTLY IF EXISTS idx_user_record_view_entity_company;
DROP INDEX CONCURRENTLY IF EXISTS idx_user_record_view_entity_contact;
