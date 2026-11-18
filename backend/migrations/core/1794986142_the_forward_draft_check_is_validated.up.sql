SET LOCAL lock_timeout = '5s';

-- The widened check admits every anchor the older one did, so each row passes.
ALTER TABLE mail_draft VALIDATE CONSTRAINT mail_draft_anchor_type;
