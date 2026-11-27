SET LOCAL lock_timeout = '3s';

-- A chat is one long thread per customer, so the thread is too coarse a unit
-- for a confidentiality question. A chat message names the burst it was judged
-- in: consecutive messages of one chat thread whose gaps are short. The
-- thread ledger keeps its key unchanged and holds a burst's question under the
-- burst id as text, so a mail thread's rows are untouched. NULL on every
-- other import.
ALTER TABLE capture_import ADD COLUMN IF NOT EXISTS burst_id uuid;
