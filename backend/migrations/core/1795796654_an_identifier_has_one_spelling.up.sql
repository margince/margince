SET LOCAL lock_timeout = '3s';

-- An address or domain a key compares has one spelling: accents composed (NFC)
-- and no character that draws nothing. values.ParseEmail and ParseDomain write
-- that spelling; these checks refuse a writer that skips them. NOT VALID here,
-- and validated by the next migration, so the scan runs under the lighter lock.
CREATE FUNCTION is_identifier_text(value text) RETURNS boolean
  LANGUAGE sql IMMUTABLE PARALLEL SAFE
  RETURN value IS NFC NORMALIZED
     AND value !~ '[\u0001-\u001F\u007F-\u009F\u00AD\u061C\u115F\u1160\u180E\u200B-\u200F\u202A-\u202E\u2060-\u2064\u2066-\u206F\u3164\uFEFF\uFFA0]';

ALTER TABLE app_user ADD CONSTRAINT app_user_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE capture_freemail_domain ADD CONSTRAINT capture_freemail_domain_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;
ALTER TABLE capture_pending_counterparty ADD CONSTRAINT capture_pending_counterparty_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE capture_sender_override ADD CONSTRAINT capture_sender_override_address_one_spelling CHECK (is_identifier_text(address)) NOT VALID;
ALTER TABLE communication_suppression ADD CONSTRAINT communication_suppression_address_one_spelling CHECK (is_identifier_text(address)) NOT VALID;
ALTER TABLE company_domain ADD CONSTRAINT company_domain_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;
ALTER TABLE company_domain_disposition ADD CONSTRAINT company_domain_disposition_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;
ALTER TABLE contact_email ADD CONSTRAINT contact_email_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE deal_room_participant ADD CONSTRAINT deal_room_participant_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE lead ADD CONSTRAINT lead_email_one_spelling CHECK (is_identifier_text(email)) NOT VALID;
ALTER TABLE workspace_email_domain ADD CONSTRAINT workspace_email_domain_domain_one_spelling CHECK (is_identifier_text(domain)) NOT VALID;
