SET LOCAL lock_timeout = '3s';
-- A forward keeps its draft on the message it forwards, beside the reply's and
-- the record's own, so writing one never overwrites the other.

ALTER TABLE mail_draft DROP CONSTRAINT IF EXISTS mail_draft_anchor_type;
ALTER TABLE mail_draft ADD CONSTRAINT mail_draft_anchor_type
    CHECK (anchor_type IN ('activity', 'forward', 'contact', 'company', 'deal', 'lead', 'project')) NOT VALID;
