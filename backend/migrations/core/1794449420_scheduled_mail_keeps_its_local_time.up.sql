ALTER TABLE scheduled_send ADD COLUMN scheduled_wall timestamp without time zone;
ALTER TABLE mail_draft ADD COLUMN scheduled_wall timestamp without time zone;
ALTER TABLE scheduled_send ADD CONSTRAINT scheduled_send_local_time_has_zone
 CHECK (scheduled_wall IS NULL OR public.valid_intent_time(scheduled_wall, scheduled_tz)) NOT VALID;
ALTER TABLE mail_draft ADD CONSTRAINT mail_draft_local_time_has_zone
 CHECK (scheduled_wall IS NULL OR public.valid_intent_time(scheduled_wall, scheduled_tz)) NOT VALID;
