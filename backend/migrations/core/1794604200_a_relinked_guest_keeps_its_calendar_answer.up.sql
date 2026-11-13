-- When a member moved a meeting participant to another record by hand. A
-- calendar's roster never retires such a row, and it still settles its answer.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_participant ADD COLUMN relinked_at timestamptz;
