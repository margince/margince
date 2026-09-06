-- Reverses the audience re-derivation only.
--
-- host_user_id is NOT cleared. The column is also written by the ordinary
-- booking path, which predates this migration, so blanking every meeting's host
-- would destroy data this migration never wrote. Leaving a recovered host in
-- place is harmless: the read path treats a named host as the row's owner,
-- which is true whether this migration wrote it or the booking path did.

UPDATE activity
   SET audience = 'workspace',
       audience_reason = NULL
 WHERE kind = 'meeting'
   AND audience = 'participants'
   AND audience_reason = 'no_counterparty';
