-- Return the seeded agent seat to active.
--
-- The down is honest about what it can and cannot restore. It reactivates the
-- row this migration retired, identified the same way — but `archived_at` is
-- cleared unconditionally, so an installation that had ALREADY archived its seat
-- before this ran comes back un-archived. That state is unrecoverable: the up
-- migration coalesces rather than overwriting precisely so it does not destroy
-- an earlier archival timestamp, but nothing records which rows it set versus
-- which already carried one.
--
-- This matters only for an operator who archived the seat by hand and then
-- rolled back, and the wrong answer there is a live seat rather than a lost one.
-- Said out loud because a down migration that quietly widens what it restores is
-- worse than one that says so.
SET LOCAL lock_timeout = '3s';

UPDATE app_user
   SET status = 'active',
       archived_at = NULL
 WHERE is_agent
   AND password_hash IS NULL
   AND email LIKE 'agent@%.gradion.local';
