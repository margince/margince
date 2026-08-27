SET LOCAL lock_timeout = '3s';

REVOKE SELECT, INSERT, DELETE, UPDATE ON TABLE person_confirm_submission FROM margince_app;
REVOKE SELECT, INSERT, DELETE, UPDATE ON TABLE confirm_token FROM margince_app;

DROP INDEX IF EXISTS ix_person_confirm_submission_open;
DROP INDEX IF EXISTS ix_person_confirm_submission_person;
DROP INDEX IF EXISTS ix_confirm_token_person;

DROP TABLE IF EXISTS person_confirm_submission;
DROP TABLE IF EXISTS confirm_token;
