-- Which deals the queue judged material and at risk, one row per deal per day.
--
-- The judgement cannot be recomputed afterwards. The material bar is the
-- median of the at-risk pipeline as it stands when the queue reads it, so
-- "was this deal material on day N" depends on every at-risk deal of that day,
-- including ones since closed, repriced or archived. The only way to answer it
-- later is to write the verdict down when it is made, which this table does.
--
-- It records the VERDICT and nothing about who was shown it: no reader, no
-- rep, no queue position. The same-day next-step figure on GET
-- /worklist/response joins it against task creation, and that is its only
-- reader.
--
-- A deal id and a day, nothing more. The row goes with its deal (ON DELETE
-- CASCADE), and the retention sweep deletes it once it is older than the
-- widest window the figure reads — the row answers a question over that
-- window and has no purpose past it.
SET LOCAL lock_timeout = '5s';

CREATE TABLE deal_risk_verdict (
    id uuid DEFAULT uuidv7() NOT NULL,
    deal_id uuid NOT NULL,
    -- The installation-zone calendar day the verdict belongs to. Stored rather
    -- than derived from created_at, because "the same day" is a question about
    -- a local day and deriving it on read would answer it in whatever zone the
    -- reading session carries.
    local_day date NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT deal_risk_verdict_pkey PRIMARY KEY (id),
    -- One verdict per deal per day. The pass runs hourly so a deal that turns
    -- at risk in the afternoon is still judged that day, and this is what makes
    -- the second and later passes add nothing for a deal already judged.
    CONSTRAINT uq_deal_risk_verdict_day UNIQUE (deal_id, local_day)
);

ALTER TABLE deal_risk_verdict
    ADD CONSTRAINT deal_risk_verdict_deal_id_fkey FOREIGN KEY (deal_id)
    REFERENCES deal(id) ON DELETE CASCADE;

-- The figure reads a window of days; the retention sweep reads by age.
CREATE INDEX idx_deal_risk_verdict_day ON deal_risk_verdict (local_day);
CREATE INDEX idx_deal_risk_verdict_created ON deal_risk_verdict (created_at);

-- The retention window for an installation that is already running.
-- consent.SeedDefaultRetentionTx plants the same row for a fresh one, at
-- bootstrap, which runs AFTER migrations — so this inserts only where a ladder
-- already exists, and a fresh database is left for the seed to plant.
INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis)
SELECT 'deal_risk_verdict', NULL, 90, 'erase', 'storage_limitation'
 WHERE EXISTS (SELECT 1 FROM retention_policy)
ON CONFLICT ON CONSTRAINT retention_policy_unique DO NOTHING;
