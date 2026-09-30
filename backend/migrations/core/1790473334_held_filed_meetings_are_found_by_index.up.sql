SET LOCAL lock_timeout = '3s';
-- The filed-meeting hold lift and the capture-health count both select
-- participants-only activity by audience_reason (compose filedMeetingHeldClause).
-- Without this they drive from capture_import, which is every captured message.
-- audience_reason is a key column rather than part of the predicate because the
-- query binds it as a parameter, which a generic plan cannot match against a
-- partial predicate. A plain build: this runner holds each migration in one
-- transaction, where CONCURRENTLY cannot run; it blocks activity writes while
-- the index builds.
CREATE INDEX idx_activity_participants_by_reason
    ON activity (audience_reason, occurred_at DESC, id)
 WHERE audience = 'participants' AND restricted_at IS NULL AND archived_at IS NULL;
