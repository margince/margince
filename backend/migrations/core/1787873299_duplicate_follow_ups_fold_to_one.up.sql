-- Duplicate follow-ups fold to one, and follow-ups nothing can ever answer
-- retire. Before the effect-level claim (automation_effect_claim) existed,
-- N enabled instances of one task-minting starter wrote N identical open
-- tasks per event, and no automation ever completed a system task once the
-- lead it chased moved on. Both repairs archive rather than complete: a
-- migration has no actor to audit a "completion" honestly, while archival
-- is the same visibility retirement any operator could perform.

SET LOCAL lock_timeout = '3s';

-- Identical open system tasks collapse to the earliest copy. Identity is
-- the runtime claim's: the words, the exact due moment, and the records
-- the task points at. due_at is compared EXACTLY, not by day — two stage
-- moves on one afternoon mint two legitimately distinct follow-ups whose
-- due moments differ by the seconds between the moves, and folding those
-- would erase a task the runtime claim keeps. captured_by = 'system'
-- rides both arms because source alone is client-writable: a task a
-- caller planted with source 'system' is a person's row, and this repair
-- must not archive a person's row.
WITH task_links AS (
    SELECT l.activity_id,
           string_agg(
               l.entity_type || ':' ||
               coalesce(l.person_id, l.organization_id, l.deal_id, l.lead_id, l.project_id)::text,
               ',' ORDER BY l.entity_type,
                            coalesce(l.person_id, l.organization_id, l.deal_id, l.lead_id, l.project_id)) AS link_identity
    FROM activity_link l
    GROUP BY l.activity_id
), copies AS (
    SELECT a.id,
           row_number() OVER (
               PARTITION BY a.subject, a.due_at, coalesce(tl.link_identity, '')
               ORDER BY a.created_at, a.id) AS copy_rank
    FROM activity a
    LEFT JOIN task_links tl ON tl.activity_id = a.id
    WHERE a.kind = 'task' AND a.source = 'system' AND a.captured_by = 'system'
      AND a.is_done = false AND a.archived_at IS NULL
)
UPDATE activity SET archived_at = now()
WHERE id IN (SELECT id FROM copies WHERE copy_rank > 1);

-- An open system task whose every linked lead has left the open pool
-- (promoted or disqualified) chases nothing: nobody works a closed lead,
-- so the reminder can only mislead. A task also linked to a lead that is
-- still open keeps standing — that half of its job remains real.
UPDATE activity a SET archived_at = now()
WHERE a.kind = 'task' AND a.source = 'system' AND a.captured_by = 'system'
  AND a.is_done = false AND a.archived_at IS NULL
  AND EXISTS (
        SELECT 1 FROM activity_link l
        JOIN lead closed ON closed.id = l.lead_id
        WHERE l.activity_id = a.id
          AND closed.status NOT IN ('new', 'contacted', 'engaged'))
  AND NOT EXISTS (
        SELECT 1 FROM activity_link l
        JOIN lead open ON open.id = l.lead_id
        WHERE l.activity_id = a.id
          AND open.status IN ('new', 'contacted', 'engaged'));
