-- What changed a deal's part in a forecast, recorded when the snapshot is
-- taken.
--
-- Movement between two snapshots has to say WHO moved a number and on what
-- evidence. audit_log carries that, but it has no correlation column a later
-- reader can join on, so reconstructing the link at read time means guessing
-- from timestamps — which is wrong exactly when two changes land in the same
-- second, the case a busy pipeline produces daily.
--
-- So the snapshot stores the reference at write time, when it is known for
-- certain. A null means the deal did not change since the previous snapshot,
-- which is a real answer and the common one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE forecast_contribution
    ADD COLUMN audit_id uuid,
    -- The approval that authorised the change, where one did. A close-date
    -- correction can require approval, and a movement report that names the
    -- actor without naming what let them act tells half the story.
    ADD COLUMN approval_id uuid;

-- No foreign key to audit_log on purpose: that table is append-only and
-- retention may age rows out from under a snapshot that outlives them. A
-- dangling reference renders as "no longer recorded", which is honest; a
-- CASCADE would delete forecast history to keep a pointer valid, and a
-- RESTRICT would stop retention from ever running.
CREATE INDEX idx_forecast_contribution_audit
    ON forecast_contribution (audit_id) WHERE audit_id IS NOT NULL;
