-- Bounded: dropping a column takes an ACCESS EXCLUSIVE lock on a table the
-- relay writes on every AI state change, so it is bounded the same way the up
-- is rather than queueing behind an open transaction forever.
SET LOCAL lock_timeout = '3s';

-- The READER comes down first. `aiactivity.Mine` selects subject_label, so a
-- deployment that ran this while the current binary was still serving would
-- answer every personal feed with a missing-column error. That ordering is the
-- deployment's to keep and not something SQL can assert — it is written here
-- because the next person to reach for this file is the one who needs to know.
ALTER TABLE ai_task_run DROP COLUMN IF EXISTS subject_label;
