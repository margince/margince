SET LOCAL lock_timeout = '3s';

ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_origin_known;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_recorded_channel_known;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_channel_iff_recorded;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_reason_by_origin;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_asked_has_due;
ALTER TABLE intro_request VALIDATE CONSTRAINT intro_request_recorded_is_introduced;
