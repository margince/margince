SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user DROP CONSTRAINT IF EXISTS app_user_email_one_spelling;
ALTER TABLE capture_freemail_domain DROP CONSTRAINT IF EXISTS capture_freemail_domain_domain_one_spelling;
ALTER TABLE capture_pending_counterparty DROP CONSTRAINT IF EXISTS capture_pending_counterparty_email_one_spelling;
ALTER TABLE capture_sender_override DROP CONSTRAINT IF EXISTS capture_sender_override_address_one_spelling;
ALTER TABLE communication_suppression DROP CONSTRAINT IF EXISTS communication_suppression_address_one_spelling;
ALTER TABLE company_domain DROP CONSTRAINT IF EXISTS company_domain_domain_one_spelling;
ALTER TABLE company_domain_disposition DROP CONSTRAINT IF EXISTS company_domain_disposition_domain_one_spelling;
ALTER TABLE contact_email DROP CONSTRAINT IF EXISTS contact_email_email_one_spelling;
ALTER TABLE deal_room_participant DROP CONSTRAINT IF EXISTS deal_room_participant_email_one_spelling;
ALTER TABLE lead DROP CONSTRAINT IF EXISTS lead_email_one_spelling;
ALTER TABLE workspace_email_domain DROP CONSTRAINT IF EXISTS workspace_email_domain_domain_one_spelling;

DROP FUNCTION is_identifier_text(text);
