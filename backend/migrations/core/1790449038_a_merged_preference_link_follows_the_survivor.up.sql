-- A merge carries the retired contact's preference links onto the survivor.
-- One that would take a slot the survivor already holds, or that names an
-- address the merge left behind archived, is marked merged_into_survivor: it
-- still withdraws and still opens the survivor's preference centre, while the
-- one-live-link-per-address index stays true.
SET LOCAL lock_timeout = '3s';

ALTER TABLE preference_token DROP CONSTRAINT preference_token_revoked_reason_check;
ALTER TABLE preference_token ADD CONSTRAINT preference_token_revoked_reason_check
  CHECK (revoked_reason IN
    ('rotated', 'expired', 'erasure', 'compromise', 'merged_predecessor', 'merged_into_survivor'));

COMMENT ON COLUMN preference_token.revoked_reason IS
  'Why the token was retired. A withdrawal made with a legacy preference token is honoured for rotated/expired/merged_into_survivor and refused for erasure/compromise/merged_predecessor. A merged_into_survivor token also still opens the survivor''s preference centre.';
