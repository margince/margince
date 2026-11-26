SET LOCAL lock_timeout = '3s';

DROP TRIGGER home_read_fence_shared_mailbox_readers ON capture_connection;
DROP TRIGGER home_read_fence_team_archived ON team;
DROP TRIGGER home_read_fence_team_membership_moved ON team_membership;
DROP TRIGGER home_read_fence_team_membership_removed ON team_membership;
