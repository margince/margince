-- Back to one company per project: the trigger reads the anchor column again,
-- which the up migration deliberately left populated for exactly this path.
-- SET LOCAL lock_timeout bounds how long the ALTER TABLEs below wait to ACQUIRE
-- their locks, not how long they hold them. Both `relationship` and `project`
-- are live tables this migration did not create, so an unbounded wait would sit
-- behind an open transaction while every writer queued behind THIS statement —
-- a deploy that stalls the product instead of failing fast and being retried.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION assert_deal_project_same_org() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.project_id IS NULL THEN
    RETURN NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM project p
    WHERE p.id = NEW.project_id
      AND p.organization_id IS NOT DISTINCT FROM NEW.organization_id
  ) THEN
    RAISE EXCEPTION 'deal and project belong to different companies'
      USING ERRCODE = 'check_violation', CONSTRAINT = 'deal_project_same_org';
  END IF;
  RETURN NULL;
END;
$$;

-- The anchor column is rebuilt from the EDGES, not left as it was found.
--
-- The legacy value is stale by construction: while several companies were
-- allowed, a company could be taken off a project and another put on, and the
-- column recorded neither. Preferring it would resurrect a company that was
-- removed and discard the one that is actually there — which reads as data
-- corruption rather than as a downgrade.
--
-- The customer edge wins, because organization_id has meant "the customer"
-- since the edge existed; failing that, the oldest live edge, because a project
-- that never named a customer still has to keep SOME company or it cannot
-- satisfy the NOT NULL below.
UPDATE project p
   SET organization_id = COALESCE(
       (SELECT r.organization_id FROM relationship r
         WHERE r.kind = 'project_company' AND r.project_id = p.id
           AND r.archived_at IS NULL AND r.role = 'customer'
         ORDER BY r.created_at, r.id LIMIT 1),
       (SELECT r.organization_id FROM relationship r
         WHERE r.kind = 'project_company' AND r.project_id = p.id AND r.archived_at IS NULL
         ORDER BY r.created_at, r.id LIMIT 1),
       p.organization_id);

-- A project this direction cannot give an anchor STOPS the downgrade; it does
-- not get deleted. A migration that destroys records to satisfy a constraint is
-- a migration that loses the one thing nobody can rebuild, and an operator who
-- sees this refusal can decide what those projects should become — which is a
-- decision, not a default.
DO $$
DECLARE orphaned int;
BEGIN
  SELECT count(*) INTO orphaned FROM project WHERE organization_id IS NULL;
  IF orphaned > 0 THEN
    RAISE EXCEPTION 'cannot revert: % project(s) have no company to anchor to. '
      'Give each one a company (or archive it) and run this again.', orphaned;
  END IF;
END $$;

ALTER TABLE project ALTER COLUMN organization_id SET NOT NULL;

DROP INDEX IF EXISTS idx_rel_company_projects;
DROP INDEX IF EXISTS idx_rel_project_companies;
DROP INDEX IF EXISTS uq_rel_project_company;
-- The edges go last, after the anchor above has been rebuilt FROM them.
DELETE FROM relationship WHERE kind = 'project_company';
ALTER TABLE relationship DROP CONSTRAINT IF EXISTS rel_project_company_shape;
ALTER TABLE relationship DROP CONSTRAINT relationship_kind_check;
ALTER TABLE relationship
  ADD CONSTRAINT relationship_kind_check CHECK (kind IN (
    'employment', 'deal_stakeholder', 'partner_of', 'referred_by',
    'co_sell_with', 'project_stakeholder'));
