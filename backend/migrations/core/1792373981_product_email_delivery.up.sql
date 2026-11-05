SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
 ADD COLUMN routine_mail_scope text CHECK (routine_mail_scope IN ('personal','team','combined')),
 ADD COLUMN routine_mail_team_id uuid REFERENCES team(id) ON DELETE SET NULL,
 ADD COLUMN weekly_delivery_day integer CHECK (weekly_delivery_day BETWEEN 1 AND 7),
 ADD COLUMN combine_routine_email boolean,
 ADD COLUMN routine_delivery_time text CHECK (routine_delivery_time ~ '^(working|([01][0-9]|2[0-3]):00)$');

ALTER TABLE comms_outbound ADD COLUMN delivery_due_at timestamptz;
CREATE INDEX comms_outbound_due ON comms_outbound (delivery_due_at) WHERE delivery_due_at IS NOT NULL AND status <> 'sent';
