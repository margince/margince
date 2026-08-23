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

-- A project created while several companies were allowed may have no anchor.
-- Give it one of its companies rather than failing the migration on a NOT NULL
-- it cannot satisfy; which one is arbitrary, and that is the information this
-- direction loses by design.
UPDATE project p
   SET organization_id = (
       SELECT r.organization_id FROM relationship r
        WHERE r.kind = 'project_company' AND r.project_id = p.id AND r.archived_at IS NULL
        ORDER BY r.created_at, r.id LIMIT 1)
 WHERE p.organization_id IS NULL;

DELETE FROM project WHERE organization_id IS NULL;
ALTER TABLE project ALTER COLUMN organization_id SET NOT NULL;

DROP INDEX IF EXISTS idx_rel_company_projects;
DROP INDEX IF EXISTS idx_rel_project_companies;
DROP INDEX IF EXISTS uq_rel_project_company;
DELETE FROM relationship WHERE kind = 'project_company';
ALTER TABLE relationship DROP CONSTRAINT IF EXISTS rel_project_company_shape;
ALTER TABLE relationship DROP CONSTRAINT relationship_kind_check;
ALTER TABLE relationship
  ADD CONSTRAINT relationship_kind_check CHECK (kind IN (
    'employment', 'deal_stakeholder', 'partner_of', 'referred_by',
    'co_sell_with', 'project_stakeholder'));
