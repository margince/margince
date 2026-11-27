SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form ADD COLUMN notify_owner boolean NOT NULL DEFAULT false;
