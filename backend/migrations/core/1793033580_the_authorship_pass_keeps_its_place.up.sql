-- Where the pass that re-judges stage evidence authorship stopped.
--
-- The pass walks live activity-backed evidence in id order, 200 at a time, and
-- within one run it carries its position in memory. A run cut short by the job
-- timeout dropped that position, so the next tick read the same prefix again
-- and a large ledger never reached its tail. The pass now starts from this id
-- and clears it at the end, which begins the next run at the first row.
--
-- The admin path is scoped to one changed domain and keeps its own position in
-- memory, so it does not read or move this row.
--
-- One row per workspace database. Bookkeeping about a pass, not audited.
SET LOCAL lock_timeout = '5s';
CREATE TABLE deal_authorship_sweep_cursor (
    singleton boolean PRIMARY KEY DEFAULT true,
    -- The last evidence row the pass inspected. NULL starts at the first row.
    after_evidence_id uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT deal_authorship_sweep_cursor_one_row CHECK (singleton)
);
