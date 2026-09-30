-- Which deals the queue judged material and at risk as each day began.
--
-- The judgement cannot be recomputed afterwards. The material bar is the
-- median of the at-risk pipeline as it stands when the queue reads it, so
-- "was this deal material on day N" depends on every at-risk deal of that day,
-- including ones since closed, repriced or archived. The only way to answer it
-- later is to write the verdict down when it is made, which these tables do.
--
-- They record the VERDICT and nothing about who was shown it: no reader, no
-- rep, no queue position. The same-day next-step figure on GET
-- /worklist/response joins the verdicts against task creation, and that is
-- their only reader.
SET LOCAL lock_timeout = '5s';

-- One row per local day: the day's FIRST successful pass, which is the only
-- pass that records anything. The figure's denominator is the set the day
-- started with, so a later pass that finds the day already here adds nothing —
-- and a first pass that found nothing material still records the day, or a
-- deal turning at risk in the afternoon would be let in by the next pass.
CREATE TABLE deal_risk_day (
    id uuid DEFAULT uuidv7() NOT NULL,
    -- The installation-zone calendar day, for uniqueness and display.
    local_day date NOT NULL,
    -- The day's bounds as instants, taken ONCE from the zone in force when
    -- the day was recorded. The task join reads these, so moving the
    -- installation to another zone later does not move which tasks fell on a
    -- day already judged.
    day_start timestamptz NOT NULL,
    day_end timestamptz NOT NULL,
    -- The instant the pass judged at: one reading of the clock, used for the
    -- at-risk selection, the pricing and the day alike.
    judged_at timestamptz NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT deal_risk_day_pkey PRIMARY KEY (id),
    CONSTRAINT uq_deal_risk_day UNIQUE (local_day),
    CONSTRAINT deal_risk_day_bounds_ordered CHECK (day_end > day_start),
    CONSTRAINT deal_risk_day_judged_within CHECK (judged_at >= day_start AND judged_at < day_end)
);

-- One row per deal the day's first pass judged material and at risk.
CREATE TABLE deal_risk_verdict (
    day_id uuid NOT NULL,
    deal_id uuid NOT NULL,

    CONSTRAINT deal_risk_verdict_pkey PRIMARY KEY (day_id, deal_id)
);

-- The verdicts go with their day, which the retention sweep ages out, and with
-- their deal.
ALTER TABLE deal_risk_verdict
    ADD CONSTRAINT deal_risk_verdict_day_id_fkey FOREIGN KEY (day_id)
    REFERENCES deal_risk_day(id) ON DELETE CASCADE;
ALTER TABLE deal_risk_verdict
    ADD CONSTRAINT deal_risk_verdict_deal_id_fkey FOREIGN KEY (deal_id)
    REFERENCES deal(id) ON DELETE CASCADE;

CREATE INDEX idx_deal_risk_verdict_deal ON deal_risk_verdict (deal_id);
CREATE INDEX idx_deal_risk_day_start ON deal_risk_day (day_start);

-- The retention window for an installation that is already running.
-- consent.SeedDefaultRetentionTx plants the same row for a fresh one, at
-- bootstrap, which runs AFTER migrations — so this inserts only where the
-- installation already exists. It asks the workspace table rather than the
-- ladder, because an admin may have deleted every policy and that ladder is
-- still an installation's.
INSERT INTO retention_policy (object_type, category, retain_days, action, lawful_basis)
SELECT 'deal_risk_day', NULL, 90, 'erase', 'storage_limitation'
 WHERE EXISTS (SELECT 1 FROM workspace)
ON CONFLICT ON CONSTRAINT retention_policy_unique DO NOTHING;
