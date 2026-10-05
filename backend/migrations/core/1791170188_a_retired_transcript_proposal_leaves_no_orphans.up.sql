-- Close the transcript proposals staged before commitments had one rule.
--
-- A meeting's next steps used to be staged as `transcript_proposal` approvals.
-- They are staged as `commitment_task` now, and the old kind's executor is gone
-- from the build. A pending approval of that kind can no longer be applied, so
-- its window is closed now: from this moment it reads as expired and refuses
-- every decision, and the expiry sweep settles it the way it settles any
-- unanswered card, with its audit row and the approval.decided event that
-- withdraws the notices about it. The next reading of the transcript stages
-- the step again under the new kind.
SET LOCAL lock_timeout = '3s';

WITH lapsed AS (
    UPDATE approval a
       SET expires_at = now(), version = a.version + 1, updated_at = now()
      FROM (SELECT id, expires_at FROM approval
             WHERE kind = 'transcript_proposal' AND status = 'pending' AND expires_at > now()
             FOR UPDATE) before
     WHERE a.id = before.id
    RETURNING a.id, before.expires_at AS was
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'approval', id,
       jsonb_build_object('expires_at', was), jsonb_build_object('expires_at', now())
  FROM lapsed;
