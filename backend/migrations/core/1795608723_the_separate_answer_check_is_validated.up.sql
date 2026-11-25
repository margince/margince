-- Validates the assurance_resolution check the previous migration added NOT VALID.
-- It is a file of its own so the scan takes only SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_resolution VALIDATE CONSTRAINT assurance_resolution_outcome_check;
