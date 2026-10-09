SET LOCAL lock_timeout = '3s';
-- A removal keeps the member's note here rather than in audit_log: erasure
-- deletes this table's rows for its subject, and audit_log is append-only.
ALTER TABLE list_member_event ADD COLUMN member_note text;
