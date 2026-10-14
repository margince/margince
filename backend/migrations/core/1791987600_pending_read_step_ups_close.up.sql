-- A read no longer has an allowance of its own, so a step-up that was asked for
-- one cannot be answered: the release path refuses a counter a human cannot
-- release, and the person deciding would meet an error instead of a choice.
-- Setting the expiry to now lets the expiry sweep close each row the ordinary
-- way, with its audit entry and event.
SET LOCAL lock_timeout = '3s';

UPDATE approval
   SET expires_at = now()
 WHERE kind = 'volume_release'
   AND status = 'pending'
   AND proposed_change->>'counter' = 'reads';
