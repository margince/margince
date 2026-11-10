-- pgmigrate:no-transaction
-- A meeting's identity key is its event UID, a slash, and the occurrence's
-- start (activities.MeetingIdentityKey). An invitation knows only the UID, so
-- it finds its meetings by the part before the last slash. The expression is
-- activities.meetingUIDOfKeySQL's own, or the planner cannot use it. The
-- first drop clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS activity_identity_meeting_uid;
CREATE INDEX CONCURRENTLY activity_identity_meeting_uid
    ON activity_identity ((left(identity_key, length(identity_key) - strpos(reverse(identity_key), '/'))))
    WHERE identity_kind = 'meeting';
