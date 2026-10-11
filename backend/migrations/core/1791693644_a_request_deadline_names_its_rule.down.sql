SET LOCAL lock_timeout = '3s';

ALTER TABLE data_subject_request
    DROP COLUMN IF EXISTS due_overridden,
    DROP COLUMN IF EXISTS due_rule;
