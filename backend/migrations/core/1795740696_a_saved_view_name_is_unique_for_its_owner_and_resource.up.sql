-- Saved views are personal settings entered by hand. Only later live twins
-- change; the oldest keeps its spelling, and archived views release names.
SET LOCAL lock_timeout = '3s';
LOCK TABLE saved_view IN SHARE ROW EXCLUSIVE MODE;
DO $twins$
DECLARE
 twin record;
 suffix bigint;
 candidate text;
BEGIN
 FOR twin IN
  SELECT * FROM (
   SELECT id, owner_id, resource, btrim(name) AS base,
          row_number() OVER (PARTITION BY owner_id, resource, lower(btrim(name)) ORDER BY created_at, id) AS rank
     FROM saved_view WHERE archived_at IS NULL
  ) ranked WHERE rank > 1 ORDER BY owner_id, resource, lower(base), rank
 LOOP
  -- A removed member cannot hold a live view; the retirement guard refuses
  -- renaming one, but permits retiring the duplicate while keeping its history.
  IF EXISTS (SELECT 1 FROM app_user WHERE id = twin.owner_id AND removed_at IS NOT NULL) THEN
   UPDATE saved_view SET archived_at = now() WHERE id = twin.id;
   CONTINUE;
  END IF;
  suffix := 2;
  LOOP
   -- 120 is saved_view.name's label bound (values.TextLabelCharacters).
   candidate := left(twin.base, 120 - length(' (' || suffix || ')')) || ' (' || suffix || ')';
   EXIT WHEN NOT EXISTS (
    SELECT 1 FROM saved_view WHERE owner_id = twin.owner_id AND resource = twin.resource
      AND archived_at IS NULL AND lower(btrim(name)) = lower(candidate)
   );
   suffix := suffix + 1;
  END LOOP;
  UPDATE saved_view SET name = candidate WHERE id = twin.id;
 END LOOP;
END
$twins$;
CREATE UNIQUE INDEX saved_view_name_once ON saved_view (owner_id, resource, lower(btrim(name))) WHERE archived_at IS NULL;
