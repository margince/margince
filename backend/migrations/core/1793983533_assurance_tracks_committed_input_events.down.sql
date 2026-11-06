SET LOCAL lock_timeout = '3s';

DROP INDEX event_outbox_event_id;
DROP INDEX event_outbox_assurance_unread;
ALTER TABLE assurance_run DROP COLUMN outbox_inputs_tracked;
ALTER TABLE event_outbox DROP COLUMN assurance_inputs_read;
