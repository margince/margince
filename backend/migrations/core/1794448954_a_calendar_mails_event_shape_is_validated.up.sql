SET LOCAL lock_timeout = '3s';
ALTER TABLE activity VALIDATE CONSTRAINT activity_calendar_event_shape;
