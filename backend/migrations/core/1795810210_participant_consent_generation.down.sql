SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_member_grant DROP COLUMN consent_version;
