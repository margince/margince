-- Validates the passport check the app caller migration added NOT VALID. It is
-- a file of its own so the scan takes only SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE passport VALIDATE CONSTRAINT passport_app_caller_pair;
