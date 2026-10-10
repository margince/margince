-- One disqualification reason per label, compared trimmed and case-folded, so a
-- rep is never offered "No budget" and " no budget " as two reasons.
--
-- An installation may already hold such twins. Each later twin is renamed, not
-- merged or deleted: leads point at it by id, and a migration has no actor to
-- answer for moving them. The built-in, else the oldest, row keeps the label.
-- A rename that would itself collide carries the row id instead.
SET LOCAL lock_timeout = '3s';

WITH ranked AS (
  SELECT id, btrim(label) AS base,
         row_number() OVER (PARTITION BY lower(btrim(label)) ORDER BY system DESC, created_at, id) AS rank
    FROM lead_disqualify_reason
), twin AS (
  SELECT id,
         base || ' (duplicate ' || (rank - 1) || ')' AS readable,
         base || ' (' || id || ')' AS unmistakable
    FROM ranked
   WHERE rank > 1
)
UPDATE lead_disqualify_reason r
   SET label = CASE WHEN EXISTS (
                 SELECT 1 FROM lead_disqualify_reason other
                  WHERE lower(btrim(other.label)) = lower(twin.readable))
               THEN twin.unmistakable ELSE twin.readable END
  FROM twin
 WHERE r.id = twin.id;

CREATE UNIQUE INDEX lead_disqualify_reason_label_once
    ON lead_disqualify_reason (lower(btrim(label)));
