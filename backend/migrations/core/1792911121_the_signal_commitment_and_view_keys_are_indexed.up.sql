-- pgmigrate:no-transaction
-- The indexes a delete of a deal, lead, contact, company or project finds its
-- signals, weekly commitments and record views by. Built concurrently, so
-- writes are never blocked for the build. Every row already there has NULL
-- keys, so each index starts empty. The drop ahead of a build clears an
-- invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_deal;
CREATE INDEX CONCURRENTLY idx_signal_entity_deal ON signal (entity_deal_id) WHERE entity_deal_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_company;
CREATE INDEX CONCURRENTLY idx_signal_entity_company ON signal (entity_company_id) WHERE entity_company_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_contact;
CREATE INDEX CONCURRENTLY idx_signal_entity_contact ON signal (entity_contact_id) WHERE entity_contact_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_signal_entity_project;
CREATE INDEX CONCURRENTLY idx_signal_entity_project ON signal (entity_project_id) WHERE entity_project_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_deal;
CREATE INDEX CONCURRENTLY idx_weekly_plan_commitment_linked_deal ON weekly_plan_commitment (linked_deal_id) WHERE linked_deal_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_lead;
CREATE INDEX CONCURRENTLY idx_weekly_plan_commitment_linked_lead ON weekly_plan_commitment (linked_lead_id) WHERE linked_lead_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_contact;
CREATE INDEX CONCURRENTLY idx_weekly_plan_commitment_linked_contact ON weekly_plan_commitment (linked_contact_id) WHERE linked_contact_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_company;
CREATE INDEX CONCURRENTLY idx_weekly_plan_commitment_linked_company ON weekly_plan_commitment (linked_company_id) WHERE linked_company_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_commitment_linked_project;
CREATE INDEX CONCURRENTLY idx_weekly_plan_commitment_linked_project ON weekly_plan_commitment (linked_project_id) WHERE linked_project_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_user_record_view_entity_company;
CREATE INDEX CONCURRENTLY idx_user_record_view_entity_company ON user_record_view (entity_company_id) WHERE entity_company_id IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_user_record_view_entity_contact;
CREATE INDEX CONCURRENTLY idx_user_record_view_entity_contact ON user_record_view (entity_contact_id) WHERE entity_contact_id IS NOT NULL;
