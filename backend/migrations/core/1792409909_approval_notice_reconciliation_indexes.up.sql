SET LOCAL lock_timeout = '3s';

CREATE INDEX notice_outstanding_kind_id ON notice (kind, id) WHERE retracted_at IS NULL;
CREATE INDEX notice_dedupe_key ON notice (dedupe_key) WHERE dedupe_key IS NOT NULL;
