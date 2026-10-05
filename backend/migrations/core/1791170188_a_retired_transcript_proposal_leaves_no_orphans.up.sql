-- Close the transcript proposals staged before commitments had one rule.
--
-- A meeting's next steps used to be staged as `transcript_proposal` approvals.
-- They are staged as `commitment_task` now, and the old kind's executor is gone
-- from the build. A pending approval of a kind nothing can execute stays in the
-- table until its window closes and is refused by every verb meanwhile, so it
-- is expired now, by the system, with the reason on the row and one audit entry
-- each: the same terminal state the expiry sweep gives an unanswered one. The
-- next reading of the transcript stages the step again under the new kind.
SET LOCAL lock_timeout = '3s';

WITH closed AS (
    UPDATE approval
       SET status = 'expired', decided_at = now(),
           decision_reason = 'next steps from a meeting are proposed as commitment tasks now; read the transcript again to propose this one',
           version = version + 1, updated_at = now()
     WHERE kind = 'transcript_proposal' AND status = 'pending'
    RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'expire', 'approval', id,
       '{"status":"pending"}'::jsonb, '{"status":"expired"}'::jsonb
  FROM closed;
