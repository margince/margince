-- Each provider's BYOK key is its own stored setting, ai.provider_keys_<provider>,
-- holding the vault ref its one map row held. The refs move unchanged: the
-- sealed blobs they name stay where they are.
SET LOCAL lock_timeout = '5s';

INSERT INTO setting (key, value)
SELECT 'ai.provider_keys_' || entry.key, to_jsonb(entry.value)
  FROM setting, jsonb_each_text(setting.value) AS entry
 WHERE setting.key = 'ai.provider_keys'
   AND jsonb_typeof(setting.value) = 'object'
   AND entry.value <> ''
ON CONFLICT (key) DO NOTHING;

DELETE FROM setting WHERE key = 'ai.provider_keys';
