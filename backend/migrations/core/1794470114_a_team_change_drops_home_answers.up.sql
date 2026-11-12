-- A team reads a shared mailbox's mail, and a selected audience through a
-- team, so a member leaving, a team archived or a mailbox moved to other
-- readers changes who reads a message. Home caches its answers until
-- home_read_fence moves, so each of these moves it.
SET LOCAL lock_timeout = '3s';

CREATE TRIGGER home_read_fence_team_membership_removed AFTER DELETE ON team_membership
    FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();

CREATE TRIGGER home_read_fence_team_membership_moved AFTER UPDATE OF team_id, user_id ON team_membership
    FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();

CREATE TRIGGER home_read_fence_team_archived AFTER UPDATE OF archived_at ON team FOR EACH ROW
    WHEN (old.archived_at IS NULL AND new.archived_at IS NOT NULL)
    EXECUTE FUNCTION advance_home_read_fence();

CREATE TRIGGER home_read_fence_shared_mailbox_readers AFTER UPDATE OF reader_team_id, mail_posture ON capture_connection
    FOR EACH ROW WHEN (old.reader_team_id IS DISTINCT FROM new.reader_team_id OR old.mail_posture IS DISTINCT FROM new.mail_posture)
    EXECUTE FUNCTION advance_home_read_fence();
