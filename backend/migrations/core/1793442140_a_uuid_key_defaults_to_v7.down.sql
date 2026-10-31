-- Back to twelve tables where the caller is the only source of the id.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_owner_identity_withdrawn ALTER COLUMN id DROP DEFAULT;
ALTER TABLE commission_entry ALTER COLUMN id DROP DEFAULT;
ALTER TABLE comms_outbound ALTER COLUMN id DROP DEFAULT;
ALTER TABLE forecast_capture_status ALTER COLUMN id DROP DEFAULT;
ALTER TABLE mail_draft ALTER COLUMN id DROP DEFAULT;
ALTER TABLE report_definition ALTER COLUMN id DROP DEFAULT;
ALTER TABLE report_edition ALTER COLUMN id DROP DEFAULT;
ALTER TABLE report_execution ALTER COLUMN id DROP DEFAULT;
ALTER TABLE report_schedule ALTER COLUMN id DROP DEFAULT;
ALTER TABLE sales_target ALTER COLUMN id DROP DEFAULT;
ALTER TABLE scheduled_send ALTER COLUMN id DROP DEFAULT;
ALTER TABLE transcript_read ALTER COLUMN id DROP DEFAULT;
