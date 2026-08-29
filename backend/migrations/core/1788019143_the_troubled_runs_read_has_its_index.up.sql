-- The Worklist's automation-health lane reads the recent failed and blocked
-- firings on every page load; workflow_run is append-only and grows without
-- bound, so the read gets the partial index its predicate names rather than
-- a scan that slows with history.
SET LOCAL lock_timeout = '3s';
CREATE INDEX workflow_run_troubled
  ON workflow_run (created_at DESC, id DESC)
  WHERE status IN ('failed', 'blocked');
