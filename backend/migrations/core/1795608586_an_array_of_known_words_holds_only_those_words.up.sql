-- 1795608586: an_array_of_known_words_holds_only_those_words.
SET LOCAL lock_timeout = '3s';

-- Each of these arrays draws from a fixed list the Go code owns, and until now
-- only the Go writer held it. A containment check closes the set in the
-- database too. backend/gates/arrayvocabulary_test.go holds each list equal to
-- its Go source. Added NOT VALID; the next file validates them.
ALTER TABLE passport ADD CONSTRAINT passport_scopes_vocabulary
    CHECK (scopes <@ ARRAY['read', 'draft', 'write', 'send', 'enrich']) NOT VALID;
ALTER TABLE oauth_grant ADD CONSTRAINT oauth_grant_scopes_vocabulary
    CHECK (scopes <@ ARRAY['read', 'draft', 'write', 'send', 'enrich']) NOT VALID;
ALTER TABLE passport ADD CONSTRAINT passport_work_areas_vocabulary
    CHECK (work_areas <@ ARRAY['business', 'compliance', 'capture', 'workspace_settings', 'admin_settings', 'own_account', 'agent_governance', 'operator', 'public']) NOT VALID;
ALTER TABLE oauth_grant ADD CONSTRAINT oauth_grant_work_areas_vocabulary
    CHECK (work_areas <@ ARRAY['business', 'compliance', 'capture', 'workspace_settings', 'admin_settings', 'own_account', 'agent_governance', 'operator', 'public']) NOT VALID;
ALTER TABLE oauth_authorization_code ADD CONSTRAINT oauth_authorization_code_work_areas_vocabulary
    CHECK (work_areas <@ ARRAY['business', 'compliance', 'capture', 'workspace_settings', 'admin_settings', 'own_account', 'agent_governance', 'operator', 'public']) NOT VALID;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_activity_kinds_vocabulary
    CHECK (activity_kinds <@ ARRAY['call', 'email', 'meeting', 'message', 'note', 'task']) NOT VALID;
ALTER TABLE deal ADD CONSTRAINT deal_manager_request_reasons_vocabulary
    CHECK (manager_request_reasons <@ ARRAY['big_deal', 'single_contact', 'stuck']) NOT VALID;
