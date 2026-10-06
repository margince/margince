-- A rule can watch a Live List and add records to a Shortlist. The rule pauses
-- itself when the list it watches is archived or broken, or when one check
-- moves more records than it acts on at once, and says why; its owner resumes
-- it. A record a rule adds to a Shortlist is recorded as the automation's.
SET LOCAL lock_timeout = '5s';

ALTER TABLE automation
    ADD COLUMN paused_reason text,
    ADD CONSTRAINT automation_paused_reason_check
        CHECK (paused_reason IN ('list_archived', 'list_invalid', 'list_unavailable', 'burst'));

ALTER TABLE list_member_event
    DROP CONSTRAINT list_member_event_reason_check,
    ADD CONSTRAINT list_member_event_reason_check CHECK (reason IN ('chosen', 'bulk', 'record_archived', 'record_restored', 'evaluated', 'filter_changed', 'automation'));
