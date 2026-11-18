SET LOCAL lock_timeout = '3s';
ALTER TABLE activity DROP COLUMN IF EXISTS forwarded_to_self;
