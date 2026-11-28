SET LOCAL lock_timeout = '5s';

ALTER TABLE activity VALIDATE CONSTRAINT activity_meeting_notes_kind;
