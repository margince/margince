SET LOCAL lock_timeout = '3s';

-- A business link approved before seats existed is connected with no seat, so
-- nothing can file its messages. Approval now names a reader team, which the
-- system cannot choose, so each such link goes back to waiting for an admin.
UPDATE channel_connection
   SET status = 'pending_approval', approved_at = NULL
 WHERE provider = 'telegram_business' AND status = 'connected' AND seat_id IS NULL;
