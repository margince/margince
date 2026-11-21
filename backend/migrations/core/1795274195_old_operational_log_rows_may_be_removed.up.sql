-- A delete is admitted only for an action with a window, and only past it.
-- Every other action, and any new one, stays until it is given a window.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION system_log_immutable() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  entry_age interval := now() - OLD.occurred_at;
BEGIN
  IF TG_OP = 'UPDATE' AND OLD.action = 'login'
     AND OLD.detail ? 'email_hash' AND NOT OLD.detail ? 'email_key_id'
     AND NEW.detail->>'email_hash' ~ '^[0-9a-f]{64}$'
     AND NEW.detail->>'email_key_id' ~ '^[0-9a-f]{16}$'
     AND (NEW.detail - ARRAY['email_hash','email_key_id']) = (OLD.detail - 'email_hash')
     AND (to_jsonb(NEW) - 'detail') = (to_jsonb(OLD) - 'detail') THEN
    RETURN NEW;
  END IF;
  IF TG_OP = 'DELETE' AND (
       (entry_age > interval '90 days' AND OLD.action IN (
          'capture_sync_error', 'capture_ensure_fault', 'capture_deal_attribution_fault',
          'capture_project_attribution_fault', 'data_task.batch_failed', 'home_repair.skipped',
          'contact_employer_repair.skipped', 'dedupe_recheck', 'dedupe_near_match',
          'comms_identity_reconcile_failed', 'ai_task.state_changed'))
    OR (entry_age > interval '400 days' AND OLD.action IN (
          'login', 'password_reset', 'password_reset_requested', 'capture.credential_rotated'))) THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'system_log is append-only (attempted % on row %)', TG_OP, OLD.id
    USING ERRCODE = 'check_violation';
END;
$$;

DROP INDEX IF EXISTS idx_system_log_actor;
