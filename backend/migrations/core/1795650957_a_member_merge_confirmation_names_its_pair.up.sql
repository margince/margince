-- A member merge of a used account waits for the holder of each side to
-- confirm it. The confirmation is a single-use mailed token in auth_token, as
-- a set-password link is, with one more column: the account on the other side
-- of the pair, and whether its own account is the survivor (true) or the source (false). A token
-- counts only for the pair and direction it names, so a confirmation of A into B
-- is not one of B into A and cannot be spent on another merge.
--
-- The checks are added NOT VALID; the migration after this one validates them
-- in a transaction of its own.
SET LOCAL lock_timeout = '3s';

ALTER TABLE auth_token ADD COLUMN IF NOT EXISTS counterpart_user_id uuid
    CONSTRAINT auth_token_counterpart_fkey REFERENCES app_user(id) ON DELETE CASCADE;

ALTER TABLE auth_token ADD COLUMN IF NOT EXISTS merge_survivor boolean;

ALTER TABLE auth_token DROP CONSTRAINT auth_token_purpose_check;

ALTER TABLE auth_token ADD CONSTRAINT auth_token_purpose_check
    CHECK (purpose IN ('password_reset', 'email_verify', 'invite', 'member_merge_confirm')) NOT VALID;

ALTER TABLE auth_token ADD CONSTRAINT auth_token_counterpart_check
    CHECK ((purpose = 'member_merge_confirm') = (counterpart_user_id IS NOT NULL)) NOT VALID;

ALTER TABLE auth_token ADD CONSTRAINT auth_token_merge_survivor_check
    CHECK ((purpose = 'member_merge_confirm') = (merge_survivor IS NOT NULL)) NOT VALID;
