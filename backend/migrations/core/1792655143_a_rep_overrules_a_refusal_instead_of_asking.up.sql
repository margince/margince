-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- A rep no longer hands a refused send to a colleague; they overrule a machine
-- refusal with their own reason. Cards raised by the old ask are closed, and
-- their reviews go back to the rep, the same ending a declined or expired card
-- had. One audit entry each.
WITH closed AS (
    UPDATE approval
       SET status = 'expired', decided_at = now(),
           decision_reason = 'asking a colleague to decide a refused send is no longer offered; overrule the refusal with a reason instead',
           version = version + 1, updated_at = now()
     WHERE kind = 'communication_review' AND status = 'pending'
    RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'expire', 'approval', id,
       '{"status":"pending"}'::jsonb, '{"status":"expired"}'::jsonb
  FROM closed;

WITH returned AS (
    UPDATE communication_review
       SET state = 'needs_context', approval_id = NULL
     WHERE state = 'awaiting_decision' AND resolved_at IS NULL
    RETURNING id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'communication_review', id,
       '{"status":"awaiting_decision"}'::jsonb, '{"status":"needs_context"}'::jsonb
  FROM returned;
