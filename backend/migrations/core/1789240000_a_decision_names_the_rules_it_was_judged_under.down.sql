-- The constraint first: dropping a column a CHECK still names fails.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_decision
  DROP CONSTRAINT IF EXISTS communication_decision_ruleset_shape;

ALTER TABLE communication_decision
  DROP COLUMN IF EXISTS ruleset_codes,
  DROP COLUMN IF EXISTS ruleset_version;
