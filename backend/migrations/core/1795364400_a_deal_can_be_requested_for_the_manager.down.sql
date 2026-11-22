SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    DROP COLUMN IF EXISTS manager_request_reasons,
    DROP COLUMN IF EXISTS manager_requested_by,
    DROP COLUMN IF EXISTS manager_requested_at;
