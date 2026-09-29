-- Close the work the retired model-price crawl left behind.
--
-- The crawl staged an `ai_model_rate_proposal` approval per changed price and ran
-- as an `ai_model_rate_refresh` job. Both are gone from the build, and neither
-- disappears by itself: a pending approval of a kind nothing can decide or
-- withdraw stays in the table until its window closes and is refused by every
-- verb meanwhile, and River keeps a queued job of a kind no worker claims,
-- retrying it into `discarded`.
--
-- Pending approvals are expired now, by the system, with the reason on the row
-- and one audit entry each, the same terminal state the expiry sweep gives an
-- unanswered one. Queued jobs are deleted; a completed row records that the work
-- ran and stays. The river_job half is guarded on the table existing, because
-- River owns its own schema and applies it on first run.
WITH closed AS (
    UPDATE approval
       SET status = 'expired', decided_at = now(),
           decision_reason = 'model prices are refreshed from the provider catalogue now; this proposal can no longer be applied',
           version = version + 1, updated_at = now()
     WHERE kind = 'ai_model_rate_proposal' AND status = 'pending'
    RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'expire', 'approval', id,
       '{"status":"pending"}'::jsonb, '{"status":"expired"}'::jsonb
  FROM closed;

DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    DELETE FROM river_job
     WHERE kind = 'ai_model_rate_refresh'
       AND state NOT IN ('completed', 'discarded', 'cancelled');
  END IF;
END
$$;
