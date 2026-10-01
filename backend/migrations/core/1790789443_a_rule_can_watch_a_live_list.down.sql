SET LOCAL lock_timeout = '5s';

UPDATE list_member_event SET reason = 'chosen' WHERE reason = 'automation';
ALTER TABLE list_member_event
    DROP CONSTRAINT IF EXISTS list_member_event_reason_check,
    ADD CONSTRAINT list_member_event_reason_check CHECK (reason IN ('chosen', 'bulk', 'record_archived', 'record_restored', 'evaluated', 'filter_changed'));

ALTER TABLE automation
    DROP CONSTRAINT IF EXISTS automation_paused_reason_check,
    DROP COLUMN IF EXISTS paused_reason;
