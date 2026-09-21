-- Dropping the column loses which merge produced a carried stop. The stops
-- themselves survive, which is what matters: the lineage is a reader's aid,
-- not the refusal.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression
  DROP COLUMN carried_from;
