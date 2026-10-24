SET LOCAL lock_timeout = '3s';

DROP INDEX intro_request_recorded_pair;
DELETE FROM intro_request WHERE origin = 'recorded';
ALTER TABLE intro_request
  DROP CONSTRAINT intro_request_recorded_is_introduced,
  DROP CONSTRAINT intro_request_asked_has_due,
  DROP CONSTRAINT intro_request_reason_by_origin,
  DROP CONSTRAINT intro_request_channel_iff_recorded,
  DROP CONSTRAINT intro_request_recorded_channel_known,
  DROP CONSTRAINT intro_request_origin_known;
ALTER TABLE intro_request ALTER COLUMN due_at SET NOT NULL;
ALTER TABLE intro_request ALTER COLUMN internal_reason SET NOT NULL;
ALTER TABLE intro_request
  ADD CONSTRAINT intro_request_reason_present CHECK (internal_reason <> '') NOT VALID;
ALTER TABLE intro_request DROP COLUMN recorded_channel, DROP COLUMN origin;
