-- Reconcile communication_decision's unique index on a database that applied
-- 1788407500 before that file was edited.
--
-- 1788407500 shipped `communication_decision_one_per_attempt` on
-- (delivery_id, recipient_address, phase, attempt). It was later edited in
-- place to ship `communication_decision_one_per_decision` on
-- (decision_set_id, recipient_address, phase). An applied version never
-- re-runs, so a database that took the first reading kept it and never saw the
-- second: the ledger says the version is done.
--
-- That is not cosmetic. Gate.AuthorizeTransmit infers on
-- ON CONFLICT (decision_set_id, recipient_address, phase), and a database
-- without a unique index matching that specification answers 42P10 —
-- on every send it evaluates, for ever.
--
-- Forward rather than by re-editing 1788407500, which would repair nothing:
-- the databases that need this are exactly the ones that will never read that
-- file again. A fresh installation already has the right index and the right
-- ledger entry, so every statement below is a no-op there.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS communication_decision_one_per_attempt;

-- If this fails with a duplicate-key error, the database holds two decisions
-- for one (decision_set_id, recipient_address, phase). The retired index did
-- not forbid that — its key carried `attempt` instead of the decision set — so
-- a drifted database can genuinely hold such a pair, and it has to be resolved
-- by hand before the index can stand. Failing loudly here is the point: the
-- alternative is a send path that keeps answering 42P10 with nothing saying why.
CREATE UNIQUE INDEX IF NOT EXISTS communication_decision_one_per_decision
    ON communication_decision (decision_set_id, recipient_address, phase);
