-- Bounded because every statement below takes an ACCESS EXCLUSIVE lock on
-- capture_exclusion, which blocks the sink's own reads: a migration that waited
-- behind a long transaction would stall capture rather than fail.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_container_is_personal;

-- REFUSE rather than delete. A container rule is a person's standing instruction
-- that a whole label's mail never enters the CRM, and the schema this rolls back
-- to cannot hold one — so a silent DELETE would resume capturing mail somebody
-- had ruled out, with nothing on any surface saying it had happened.
--
-- The operator's remedy is on the surface that owns the rules: lift them, then
-- roll back. That is a decision a person makes about their own mailbox, and it
-- is not this migration's to make for them.
DO $$
DECLARE rules int;
BEGIN
  SELECT count(*) INTO rules FROM capture_exclusion WHERE kind = 'container';
  IF rules > 0 THEN
    RAISE EXCEPTION 'refusing to roll back: % capture exclusion(s) name a container, and the '
      'earlier schema cannot hold one. Rolling back would silently resume capturing mail their '
      'owners ruled out. Lift them first (Settings -> Capture -> Exclusions), then roll back.',
      rules;
  END IF;
END $$;

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_value_check;
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_value_check
    CHECK (value = lower(value) AND value <> '');

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_kind_check;
ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_kind_check
    CHECK (kind IN ('address', 'domain'));
