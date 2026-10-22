SET LOCAL lock_timeout = '5s';

INSERT INTO setting (key, value)
SELECT 'ai.provider_keys', jsonb_object_agg(substr(key, length('ai.provider_keys_') + 1), value)
  FROM setting
 WHERE key LIKE 'ai.provider\_keys\_%'
HAVING count(*) > 0
ON CONFLICT (key) DO NOTHING;

DELETE FROM setting WHERE key LIKE 'ai.provider\_keys\_%';
