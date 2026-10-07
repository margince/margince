SET LOCAL lock_timeout = '3s';
-- The rows the upgrade copied are the ones its audit rows name and nobody has
-- edited since: a copied row is new, so any other audit row on it is an edit,
-- and a Vertex price an admin touched afterwards is theirs and stays.
DELETE FROM ai_model_rate r
 USING audit_log a
 WHERE a.entity_type = 'ai_model_rate' AND a.entity_id = r.id
   AND a.actor_type = 'system' AND a.actor_id = 'migration'
   AND a.after->>'copied_from' = 'gemini'
   AND NOT EXISTS (
     SELECT 1 FROM audit_log later
      WHERE later.entity_type = 'ai_model_rate' AND later.entity_id = r.id AND later.id <> a.id);
