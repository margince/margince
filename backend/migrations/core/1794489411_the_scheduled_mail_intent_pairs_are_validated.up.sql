SET LOCAL lock_timeout = '2s';
ALTER TABLE scheduled_send VALIDATE CONSTRAINT scheduled_send_local_time_has_zone;
ALTER TABLE mail_draft VALIDATE CONSTRAINT mail_draft_local_time_has_zone;
