SET LOCAL lock_timeout = '3s';

ALTER TABLE deal DROP CONSTRAINT IF EXISTS deal_manager_request_reasons_vocabulary;
ALTER TABLE custom_field DROP CONSTRAINT IF EXISTS custom_field_activity_kinds_vocabulary;
ALTER TABLE oauth_authorization_code DROP CONSTRAINT IF EXISTS oauth_authorization_code_work_areas_vocabulary;
ALTER TABLE oauth_grant DROP CONSTRAINT IF EXISTS oauth_grant_work_areas_vocabulary;
ALTER TABLE passport DROP CONSTRAINT IF EXISTS passport_work_areas_vocabulary;
ALTER TABLE oauth_grant DROP CONSTRAINT IF EXISTS oauth_grant_scopes_vocabulary;
ALTER TABLE passport DROP CONSTRAINT IF EXISTS passport_scopes_vocabulary;
