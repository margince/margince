SET LOCAL lock_timeout = '3s';

DELETE FROM retention_policy WHERE action = 'expire';

ALTER TABLE retention_policy DROP CONSTRAINT retention_policy_action_check;

ALTER TABLE retention_policy ADD CONSTRAINT retention_policy_action_check
    CHECK (action IN ('archive', 'anonymize', 'erase')) NOT VALID;

DROP TABLE extension_expired_row;

DROP TABLE extension_legal_hold;
