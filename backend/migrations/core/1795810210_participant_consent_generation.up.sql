SET LOCAL lock_timeout = '5s';

ALTER TABLE chat_member_grant ADD COLUMN consent_version bigint NOT NULL DEFAULT 1
 CHECK (consent_version BETWEEN 1 AND 9007199254740991);
UPDATE chat_member_grant SET consent_version=version;
