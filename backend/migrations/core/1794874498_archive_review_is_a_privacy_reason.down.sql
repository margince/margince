SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION audit_privacy_reason_codes() RETURNS text[] LANGUAGE sql IMMUTABLE
AS $$SELECT ARRAY['retention','test','dsr_request','restriction_expired','the website lead could not be sent its privacy notice','subject_data_removed','legal hold ended: the postponed content erasure was completed','retention: the record''s content was erased','controller release: the record''s content was erased','noise disposition: the content of the message was erased']::text[]$$;
