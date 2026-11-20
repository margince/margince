-- An outbound message keeps the consent purpose its sender declared and that
-- purpose's class at send time. The engine still allows a declared class, so a
-- count of sends per purpose is how a misclassified send shows afterwards.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN IF NOT EXISTS consent_purpose text,
    ADD COLUMN IF NOT EXISTS consent_class text;
