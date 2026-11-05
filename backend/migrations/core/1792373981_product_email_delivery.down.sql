SET LOCAL lock_timeout = '3s';

DROP INDEX comms_outbound_due;
ALTER TABLE comms_outbound DROP COLUMN delivery_due_at;
ALTER TABLE app_user
 DROP COLUMN routine_delivery_time,
 DROP COLUMN combine_routine_email,
 DROP COLUMN weekly_delivery_day,
 DROP COLUMN routine_mail_team_id,
 DROP COLUMN routine_mail_scope;
