-- A claim is filed once per contact, message and piece of evidence.
--
-- An extractor reads the same conversation again whenever it moves, and the
-- claim it files is how a human's dismissal is remembered. Without this index
-- a second reading files a second claim beside the dismissed one, and the
-- promise comes back. Live duplicates already stored are archived first. The
-- one kept is the one a human acted on — settled, corrected or tied to a task —
-- and otherwise the oldest; each archived row is audited.
SET LOCAL lock_timeout = '3s';

WITH ranked AS (
    SELECT id, contact_id, row_number() OVER (
             PARTITION BY contact_id, source_activity_id, evidence_fingerprint
             ORDER BY (status <> 'open') DESC, (corrected_at IS NOT NULL) DESC,
                      (task_activity_id IS NOT NULL) DESC, created_at, id) AS rank
      FROM conversation_claim
     WHERE archived_at IS NULL
), archived AS (
    UPDATE conversation_claim c
       SET archived_at = now()
      FROM ranked
     WHERE c.id = ranked.id AND ranked.rank > 1
    RETURNING c.id, c.contact_id
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'update', 'contact', contact_id,
       jsonb_build_object('claim_id', id, 'archived', false),
       jsonb_build_object('claim_id', id, 'archived', true)
  FROM archived;

CREATE UNIQUE INDEX conversation_claim_evidence_ux
    ON conversation_claim (contact_id, source_activity_id, evidence_fingerprint)
 WHERE archived_at IS NULL;
