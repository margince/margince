-- Whether an AI model rewrote the subject or body before the activity was
-- saved, so a reader sees the words as AI-assisted. Kept through later edits.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN ai_assisted boolean NOT NULL DEFAULT false;
