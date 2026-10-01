-- The narrower CHECK has no merged_into_survivor, so those rows fall back to
-- merged_predecessor: the old code refuses both, and refusing is the safe
-- reading of a link it does not understand.
SET LOCAL lock_timeout = '3s';

UPDATE preference_token SET revoked_reason = 'merged_predecessor'
 WHERE revoked_reason = 'merged_into_survivor';
ALTER TABLE preference_token DROP CONSTRAINT preference_token_revoked_reason_check;
ALTER TABLE preference_token ADD CONSTRAINT preference_token_revoked_reason_check
  CHECK (revoked_reason IN
    ('rotated', 'expired', 'erasure', 'compromise', 'merged_predecessor'));

COMMENT ON COLUMN preference_token.revoked_reason IS
  'Why the token was retired. A withdrawal made with a legacy preference token is honoured for rotated/expired and refused for erasure/compromise/merged_predecessor.';
