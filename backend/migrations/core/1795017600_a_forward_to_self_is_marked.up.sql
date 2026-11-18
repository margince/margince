-- Mail a seat forwarded to their own address is filed under the customer it
-- carries, but it is no exchange with them: the reply and engagement readers
-- skip a marked row. A constant default adds the column without a rewrite.
SET LOCAL lock_timeout = '3s';
ALTER TABLE activity ADD COLUMN IF NOT EXISTS forwarded_to_self boolean NOT NULL DEFAULT false;
