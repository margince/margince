-- The operational budgets stop being called quotas.
--
-- Until the sales quota was retired, one word named two unrelated things: the
-- revenue target a manager typed, and the volume ceiling that stops an agent
-- reading, writing or spending without bound. The first is gone. The second is
-- a safety limit and stays exactly as it was -- this migration renames what it
-- is CALLED, and changes no behaviour, no threshold and no refusal.
--
-- ONE STORED VALUE, in two tables, free text with no CHECK constraint to drop
-- and re-add. The AI provider's own sentinel is NOT renamed: 'provider_quota'
-- names the VENDOR's quota, and calling it something else here would hide
-- which system refused.
--
--   approval.kind = 'quota_release' names the offer a human decides when an
--   agent has reached its ceiling. approval_autonomy_policy.kind carries the
--   same string as half of its (user, kind) grain, and every decision writes a
--   row there -- so renaming one and not the other would split a rep's
--   history from their future counts at the moment of the deploy.
----
-- ROLLING DEPLOY. An old replica still writing 'quota_release' after this runs
-- produces a row the new code does not recognise as decidable; the card sits
-- until the rollout finishes rather than being decided wrongly. That is the
-- safe direction, and the window is one rollout.
SET LOCAL lock_timeout = '3s';

UPDATE approval SET kind = 'volume_release' WHERE kind = 'quota_release';

-- The autonomy grain is UNIQUE on (user_id, kind), so a rep who somehow holds
-- both spellings would collide on a bare rename (uq_approval_autonomy_policy_user_kind,
-- 1787806660). Fold the old row's three counters into the new one where both
-- exist, drop the folded row, then rename what is left.
UPDATE approval_autonomy_policy AS fresh
   SET approved_clean  = fresh.approved_clean  + stale.approved_clean,
       approved_edited = fresh.approved_edited + stale.approved_edited,
       rejected        = fresh.rejected        + stale.rejected
  FROM approval_autonomy_policy AS stale
 WHERE stale.user_id = fresh.user_id
   AND stale.kind = 'quota_release'
   AND fresh.kind = 'volume_release';

DELETE FROM approval_autonomy_policy
 WHERE kind = 'quota_release'
   AND EXISTS (SELECT 1 FROM approval_autonomy_policy other
                WHERE other.user_id = approval_autonomy_policy.user_id
                  AND other.kind = 'volume_release');

UPDATE approval_autonomy_policy SET kind = 'volume_release' WHERE kind = 'quota_release';
