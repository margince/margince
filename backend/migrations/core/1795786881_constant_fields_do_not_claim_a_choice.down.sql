SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_review_response ADD COLUMN revision integer NOT NULL DEFAULT 1 CHECK (revision >= 1);
ALTER TABLE communication_review ADD COLUMN kind text NOT NULL DEFAULT 'single' CONSTRAINT communication_review_kind CHECK (kind = 'single');
ALTER TABLE saved_view ADD COLUMN shared_scope text NOT NULL DEFAULT 'private' CHECK (shared_scope IN ('private', 'team', 'workspace'));
ALTER TABLE conversation_claim ADD COLUMN needs_review boolean NOT NULL DEFAULT false;
