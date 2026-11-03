SET LOCAL lock_timeout = '3s';
ALTER TABLE email_signature ADD COLUMN html text NOT NULL DEFAULT '';
