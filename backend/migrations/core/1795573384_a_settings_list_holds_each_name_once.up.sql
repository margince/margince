-- A retired lookup entry keeps its name because it can be restored; an archived
-- team, pipeline, stage or document set cannot, so it releases its name.
-- A managed document set stays out, so a user's set never makes the boot seed fail.
-- Stored twins are renamed, not merged: records point at them by id.
SET LOCAL lock_timeout = '3s';

DO $twins$
DECLARE
 list record;
BEGIN
 FOR list IN
  SELECT * FROM (VALUES
   ('deal_acquisition_source', 'label', NULL, 'true', 'system DESC, created_at, id'),
   ('deal_loss_reason', 'label', NULL, 'true', 'system DESC, created_at, id'),
   ('partner_stage', 'label', NULL, 'true', 'system DESC, created_at, id'),
   ('record_role', 'label', NULL, 'true', 'system DESC, created_at, id'),
   ('lead_origin', 'label', NULL, 'true', 'system DESC, created_at, id'),
   ('team', 'name', NULL, 'archived_at IS NULL', 'created_at, id'),
   ('pipeline', 'name', NULL, 'archived_at IS NULL', 'is_default DESC, created_at, id'),
   ('stage', 'name', 'pipeline_id', 'archived_at IS NULL', 'created_at, id'),
   ('knowledge_corpus', 'name', NULL, 'archived_at IS NULL AND managed_source IS NULL', 'created_at, id')
  ) AS target(relation, name_column, scope_column, held, keeper_first)
 LOOP
  EXECUTE format($sql$
   WITH ranked AS (
     SELECT id, btrim(%2$I) AS base, %3$s AS scope,
            row_number() OVER (PARTITION BY %3$s, lower(btrim(%2$I)) ORDER BY %5$s) AS rank
       FROM %1$I
      WHERE %4$s
   ), twin AS (
     SELECT id, scope,
            left(base, 120 - length(' (duplicate ' || (rank - 1) || ')')) || ' (duplicate ' || (rank - 1) || ')' AS readable,
            left(base, 81) || ' (' || id || ')' AS unmistakable
       FROM ranked
      WHERE rank > 1
   ), renamed AS (
     SELECT id,
            CASE WHEN count(*) OVER (PARTITION BY scope, lower(readable)) > 1
                   OR EXISTS (SELECT 1 FROM %1$I other
                               WHERE %6$s = twin.scope AND lower(btrim(other.%2$I)) = lower(twin.readable))
                 THEN unmistakable ELSE readable END AS name
       FROM twin
   )
   UPDATE %1$I target_row
      SET %2$I = renamed.name
     FROM renamed
    WHERE target_row.id = renamed.id
  $sql$,
   list.relation, list.name_column,
   CASE WHEN list.scope_column IS NULL THEN $$''$$ ELSE format('%I::text', list.scope_column) END,
   list.held, list.keeper_first,
   CASE WHEN list.scope_column IS NULL THEN $$''$$ ELSE format('other.%I::text', list.scope_column) END);
 END LOOP;
END
$twins$;

CREATE UNIQUE INDEX deal_acquisition_source_label_once ON deal_acquisition_source (lower(btrim(label)));
CREATE UNIQUE INDEX deal_loss_reason_label_once ON deal_loss_reason (lower(btrim(label)));
CREATE UNIQUE INDEX partner_stage_label_once ON partner_stage (lower(btrim(label)));
CREATE UNIQUE INDEX record_role_label_once ON record_role (lower(btrim(label)));
CREATE UNIQUE INDEX lead_origin_label_once ON lead_origin (lower(btrim(label)));

DROP INDEX team_name_unique;
CREATE UNIQUE INDEX team_name_unique ON team (lower(btrim(name))) WHERE archived_at IS NULL;

DROP INDEX pipeline_name_unique;
CREATE UNIQUE INDEX pipeline_name_unique ON pipeline (lower(btrim(name))) WHERE archived_at IS NULL;

CREATE UNIQUE INDEX stage_name_once ON stage (pipeline_id, lower(btrim(name))) WHERE archived_at IS NULL;

CREATE UNIQUE INDEX knowledge_corpus_name_once ON knowledge_corpus (lower(btrim(name)))
 WHERE archived_at IS NULL AND managed_source IS NULL;
