SET LOCAL lock_timeout = '3s';

-- The workspace table still holds every fact these keys copy, so a rollback
-- only removes the copies.
DELETE FROM installation_state
 WHERE key IN ('stored_key_workspace', 'installation_trial', 'archived_predecessor');
