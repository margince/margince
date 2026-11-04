-- A signal's kind may be one an extension declares, spelled `ext:<unit>:<kind>`.
--
-- The new check keeps core's sixteen kinds as a closed list and adds the one
-- spelling. Which `ext:` names exist is decided by the composed set at boot, not
-- by the database, as it is for every other column that admits the spelling.
--
-- It is added NOT VALID, which takes the lock without a scan, and validated in
-- the next migration: the runner wraps each file in one transaction, so a
-- VALIDATE in this file would run under the lock this statement took. The old
-- check stays until that migration swaps them, so `signal` is never left
-- without a kind constraint.
SET LOCAL lock_timeout = '3s';

ALTER TABLE signal ADD CONSTRAINT signal_kind_check_v3
    CHECK (kind IN ('stalled_deal', 'champion_left', 'reengagement', 'buying_intent',
                    'risk', 'other', 'contract_ended', 'new_opportunity',
                    'commitment_made', 'ghosted_thread', 'project_gone_quiet',
                    'funding', 'leadership_change', 'expansion', 'product_launch',
                    'technical_change')
           OR kind ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$')
    NOT VALID;
