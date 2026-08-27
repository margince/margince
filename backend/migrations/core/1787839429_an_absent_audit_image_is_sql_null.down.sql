-- Dropping the constraint restores nothing and destroys nothing: it never
-- rewrote a row, and the rows it refused were never written.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log DROP CONSTRAINT IF EXISTS audit_log_images_are_absent_or_present;
