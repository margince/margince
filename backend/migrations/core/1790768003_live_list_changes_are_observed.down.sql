SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS list_visit;
DROP TABLE IF EXISTS list_evaluation;
DROP TABLE IF EXISTS list_live_member;
DELETE FROM list_member_event WHERE action IN ('entered', 'left');
ALTER TABLE list_member_event
    DROP CONSTRAINT IF EXISTS list_member_event_reason_check,
    ADD CONSTRAINT list_member_event_reason_check CHECK (reason IN ('chosen', 'bulk', 'record_archived', 'record_restored')),
    DROP CONSTRAINT IF EXISTS list_member_event_action_check,
    ADD CONSTRAINT list_member_event_action_check CHECK (action IN ('added', 'removed')),
    DROP COLUMN IF EXISTS definition_version;
