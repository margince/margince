SET LOCAL lock_timeout = '3s';
-- The older check has no place for a forward's draft, so those drafts go first.

DELETE FROM mail_draft WHERE anchor_type = 'forward';
ALTER TABLE mail_draft DROP CONSTRAINT IF EXISTS mail_draft_anchor_type;
ALTER TABLE mail_draft ADD CONSTRAINT mail_draft_anchor_type
    CHECK (anchor_type IN ('activity', 'contact', 'company', 'deal', 'lead', 'project')) NOT VALID;
