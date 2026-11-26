-- pgmigrate:no-transaction
SET lock_timeout = '2s';
-- No statement filters, joins or orders by what these indexes hold, and no
-- foreign key needs one. Each cost a write on every row of its table.
DROP INDEX CONCURRENTLY IF EXISTS idx_assurance_exception_owner;
DROP INDEX CONCURRENTLY IF EXISTS assurance_task_item_open_in_cycle;
DROP INDEX CONCURRENTLY IF EXISTS automation_effect_claim_unapplied;
DROP INDEX CONCURRENTLY IF EXISTS idx_bulk_operation_created;
DROP INDEX CONCURRENTLY IF EXISTS idx_bulk_operation_requested_by;
DROP INDEX CONCURRENTLY IF EXISTS capture_sender_override_user_idx;
DROP INDEX CONCURRENTLY IF EXISTS close_date_run_open;
DROP INDEX CONCURRENTLY IF EXISTS communication_instruction_live;
DROP INDEX CONCURRENTLY IF EXISTS communication_review_open;
DROP INDEX CONCURRENTLY IF EXISTS idx_domain_disposition_admission;
DROP INDEX CONCURRENTLY IF EXISTS idx_domain_disposition_unevidenced;
DROP INDEX CONCURRENTLY IF EXISTS contract_renewal_due_ix;
DROP INDEX CONCURRENTLY IF EXISTS deal_won_without_contract_ix;
DROP INDEX CONCURRENTLY IF EXISTS ix_deal_room_engagement_participant;
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_credits_ix;
DROP INDEX CONCURRENTLY IF EXISTS idx_forecast_contribution_audit;
DROP INDEX CONCURRENTLY IF EXISTS idx_list_evaluation_due;
DROP INDEX CONCURRENTLY IF EXISTS idx_team_weekly_review_team;
DROP INDEX CONCURRENTLY IF EXISTS idx_technical_lookup_cache_expiry;
DROP INDEX CONCURRENTLY IF EXISTS idx_voice_corpus_profile;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_plan_owner;
DROP INDEX CONCURRENTLY IF EXISTS idx_weekly_review_user;
RESET lock_timeout;
