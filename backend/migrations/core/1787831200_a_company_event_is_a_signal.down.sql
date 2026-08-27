-- Same bound as the up: the constraint swap takes an ACCESS EXCLUSIVE lock on a
-- table the signal scan writes on every pass.
--
-- Rows carrying one of the four kinds would fail the narrowed constraint, so
-- they are retired first. `archived_at` rather than a delete: a signal somebody
-- has already read and acted on is history, and this direction is a schema
-- rollback rather than a decision that the finding was wrong.
SET LOCAL lock_timeout = '3s';

UPDATE signal
   SET kind = 'other', archived_at = coalesce(archived_at, now())
 WHERE kind IN ('funding', 'leadership_change', 'expansion', 'product_launch');

ALTER TABLE signal ADD CONSTRAINT signal_kind_check_v1
    CHECK (kind IN ('stalled_deal', 'champion_left', 'reengagement', 'buying_intent',
                    'risk', 'other', 'contract_ended', 'new_opportunity',
                    'commitment_made', 'ghosted_thread', 'project_gone_quiet'))
    NOT VALID;

ALTER TABLE signal VALIDATE CONSTRAINT signal_kind_check_v1;

ALTER TABLE signal DROP CONSTRAINT signal_kind_check;

ALTER TABLE signal RENAME CONSTRAINT signal_kind_check_v1 TO signal_kind_check;
