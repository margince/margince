SET LOCAL lock_timeout = '3s';
-- A logged message or a bound identity still names the row, so it stays then.
DELETE FROM channel_provider p
 WHERE p.provider = 'linkedin' AND p.transport = 'core'
   AND NOT EXISTS (SELECT 1 FROM activity a WHERE a.channel_provider = p.provider)
   AND NOT EXISTS (SELECT 1 FROM contact_channel_identity c WHERE c.provider = p.provider);
