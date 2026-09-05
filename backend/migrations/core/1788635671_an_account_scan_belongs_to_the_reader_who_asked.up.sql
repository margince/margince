-- The account scan: the model's reading of one account for one reader,
-- queued when the account page is opened and kept until the account moves.
--
-- One row per (reader, account), like org_brief: the input is the reader's
-- own composite read plus the message words their audience admits, so a
-- colleague's row would disclose records this reader cannot open. The row is
-- also the job's durable carrier — status is the rail's own vocabulary, so
-- the rail line and the page never disagree about where the read stands —
-- and it keeps the LAST settled findings while a new read runs, because a
-- page that emptied its advice every time the account moved would be blank
-- exactly when the account is busiest.
--
-- No workspace column and no policy: one installation holds one organization,
-- and the reader predicate is written into every statement explicitly.
SET LOCAL lock_timeout = '5s';

CREATE TABLE org_scan (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    organization_id uuid NOT NULL,
    status text NOT NULL,
    attempt integer DEFAULT 1 NOT NULL,
    requested_at timestamptz DEFAULT now() NOT NULL,
    started_at timestamptz,
    finished_at timestamptz,
    next_attempt_at timestamptz,
    fingerprint text,
    generated_at timestamptz,
    generated_by text,
    degrade_reason text,
    read_exchanges integer,
    read_deals integer,
    findings jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT org_scan_pkey PRIMARY KEY (id),
    CONSTRAINT org_scan_user_id_organization_id_key UNIQUE (user_id, organization_id),
    CONSTRAINT org_scan_status_check CHECK (status IN ('queued', 'running', 'done', 'degraded', 'failed')),
    CONSTRAINT org_scan_generated_by_check CHECK (generated_by IS NULL OR generated_by IN ('model', 'deterministic')),
    CONSTRAINT org_scan_org_fkey FOREIGN KEY (organization_id) REFERENCES organization(id) ON DELETE CASCADE,
    CONSTRAINT org_scan_user_id_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
);

COMMENT ON COLUMN org_scan.status IS
  'Where the CURRENT read stands, in the AI activity rail''s own vocabulary. The findings columns describe the last read that settled, which may be an earlier one.';
COMMENT ON COLUMN org_scan.fingerprint IS
  'The input the stored findings were read from: the reader''s composite read, the message words, the prompt and the routing. NULL until a read has settled. A mismatch is what makes a stored scan stale.';
COMMENT ON COLUMN org_scan.next_attempt_at IS
  'When a read the AI budget deferred will try again. NULL unless the read is waiting on budget.';

CREATE INDEX org_scan_organization_ix ON org_scan USING btree (organization_id);

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE org_scan TO margince_app;
