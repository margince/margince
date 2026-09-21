SET LOCAL lock_timeout = '3s';
ALTER TABLE notice
  DROP CONSTRAINT notice_target_paired,
  DROP COLUMN target_id,
  DROP COLUMN target_type;
