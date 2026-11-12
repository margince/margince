-- Put the closed check back beside the one that admits extension kinds, the
-- state the previous migration left. It is added NOT VALID, which takes the
-- lock without a scan, and stays unvalidated: no ALTER here may scan under it.
--
-- Rows carrying an extension kind would fail the closed check, so they are
-- retired first. `archived_at` rather than a delete: a signal somebody has
-- already read and acted on is history, and this direction is a schema
-- rollback rather than a decision that the finding was wrong.
SET LOCAL lock_timeout = '3s';

UPDATE signal
   SET kind = 'other', archived_at = coalesce(archived_at, now())
 WHERE kind LIKE 'ext:%';

ALTER TABLE signal RENAME CONSTRAINT signal_kind_check TO signal_kind_check_v3;

ALTER TABLE signal ADD CONSTRAINT signal_kind_check
    CHECK (kind IN ('stalled_deal', 'champion_left', 'reengagement', 'buying_intent',
                    'risk', 'other', 'contract_ended', 'new_opportunity',
                    'commitment_made', 'ghosted_thread', 'project_gone_quiet',
                    'funding', 'leadership_change', 'expansion', 'product_launch',
                    'technical_change'))
    NOT VALID;
