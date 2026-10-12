-- The table carries foreign keys OUT to contact and lead: dropping it takes a
-- lock on those too, and an unbounded wait would queue behind any open
-- transaction and stall every write to either spine for as long as this is
-- willing to wait. Three seconds, then fail and let the operator retry,
-- rather than take the estate down to undo a feature.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS communication_override;
