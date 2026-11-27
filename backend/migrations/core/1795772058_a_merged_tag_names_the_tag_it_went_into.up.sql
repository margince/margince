-- 1795772058: a_merged_tag_names_the_tag_it_went_into.
SET LOCAL lock_timeout = '3s';

-- The tag a merge folded this one into, the redirect a merged contact, company,
-- deal and lead already carry. NULL on every tag retired any other way.
ALTER TABLE tag ADD COLUMN IF NOT EXISTS merged_into_id uuid
    REFERENCES tag(id) ON DELETE SET NULL (merged_into_id);

-- A tag merged before the column existed names its survivor only in the
-- merge's audit row. It takes it when that row is still the last change to its
-- archive state: a merge, a restore (archived_at) or a retirement (no after
-- image). A later rename or link row leaves the tag merged, so it does not count.
UPDATE tag SET merged_into_id = (last_word.after ->> 'merged_into')::uuid
  FROM (SELECT archived.id,
               (SELECT a.after FROM audit_log a
                 WHERE a.entity_type = 'tag' AND a.entity_id = archived.id
                   AND (a.after IS NULL OR a.after ? 'merged_into' OR a.after ? 'archived_at')
                 ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1) AS after
          FROM tag archived
         WHERE archived.archived_at IS NOT NULL) last_word
 WHERE tag.id = last_word.id
   AND last_word.after ? 'merged_into'
   AND EXISTS (SELECT 1 FROM tag kept WHERE kept.id = (last_word.after ->> 'merged_into')::uuid);
