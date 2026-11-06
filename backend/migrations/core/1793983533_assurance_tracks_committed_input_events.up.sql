SET LOCAL lock_timeout = '3s';

ALTER TABLE event_outbox ADD COLUMN assurance_inputs_read boolean NOT NULL DEFAULT true;
ALTER TABLE event_outbox ALTER COLUMN assurance_inputs_read SET DEFAULT false;
ALTER TABLE assurance_run ADD COLUMN outbox_inputs_tracked boolean NOT NULL DEFAULT false;

CREATE INDEX event_outbox_assurance_unread ON event_outbox ((envelope->>'type'))
    WHERE NOT assurance_inputs_read;
CREATE INDEX event_outbox_event_id ON event_outbox ((envelope->>'event_id'));
