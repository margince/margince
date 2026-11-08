SET LOCAL lock_timeout = '3s';
ALTER TABLE mail_draft ADD COLUMN editor_state jsonb NOT NULL DEFAULT '{}'::jsonb;
