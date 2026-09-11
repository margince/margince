SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS data_subject_request_receipt_reference;
DROP INDEX IF EXISTS data_subject_request_one_case_per_submission;
ALTER TABLE data_subject_request DROP CONSTRAINT IF EXISTS data_subject_request_channel;
ALTER TABLE data_subject_request DROP COLUMN IF EXISTS receipt_reference;
ALTER TABLE data_subject_request DROP COLUMN IF EXISTS source_submission_id;
ALTER TABLE data_subject_request DROP COLUMN IF EXISTS channel;
ALTER TABLE data_subject_request DROP COLUMN IF EXISTS received_at;
ALTER TABLE data_subject_request DROP COLUMN IF EXISTS person_id;
