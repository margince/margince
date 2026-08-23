-- A project is work several companies do together, not work one company owns.
--
-- CompA, CompB and CompC building ProjectA is the ordinary case, not an
-- exception: a delivery has a customer, a partner and a subcontractor on it,
-- and a model that admits only one of them forces the other two out of the
-- record they are working in. project.organization_id could carry only one, so
-- the companies move onto an edge.
--
-- The edge is a `relationship` row, not a new table. That table already models
-- project↔person (kind project_stakeholder) with the same shape — a role, a
-- source, the audit columns, the archive semantics — and a second table would
-- be a second answer to "who is on this project".

-- SET LOCAL lock_timeout bounds how long the ALTER TABLEs below wait to ACQUIRE
-- their locks, not how long they hold them. Both `relationship` and `project`
-- are live tables this migration did not create, so an unbounded wait would sit
-- behind an open transaction while every writer queued behind THIS statement —
-- a deploy that stalls the product instead of failing fast and being retried.
SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship
  DROP CONSTRAINT relationship_kind_check;

ALTER TABLE relationship
  ADD CONSTRAINT relationship_kind_check CHECK (kind IN (
    'employment', 'deal_stakeholder', 'partner_of', 'referred_by',
    'co_sell_with', 'project_stakeholder', 'project_company'));

-- A project_company edge names a project and a company and nothing else, the
-- same way the stakeholder shape names a project and a person.
ALTER TABLE relationship
  ADD CONSTRAINT rel_project_company_shape CHECK (
    (kind <> 'project_company') OR (
      project_id IS NOT NULL AND organization_id IS NOT NULL
      AND person_id IS NULL AND counterparty_org_id IS NULL AND deal_id IS NULL));

-- One live edge per (project, company): naming the same company twice on one
-- project is not two facts, and a role change is an update to the edge that
-- exists.
CREATE UNIQUE INDEX uq_rel_project_company
  ON relationship (project_id, organization_id)
  WHERE kind = 'project_company' AND archived_at IS NULL;

CREATE INDEX idx_rel_project_companies
  ON relationship (project_id)
  WHERE kind = 'project_company' AND archived_at IS NULL;

CREATE INDEX idx_rel_company_projects
  ON relationship (organization_id)
  WHERE kind = 'project_company' AND archived_at IS NULL;

-- Every project that exists keeps the company it has, as a `customer` edge.
-- Without this half a deployed installation would come back from the migration
-- with every project's company silently gone from the surfaces that read the
-- edge — the anchor column still holds it, but nothing reads that column any
-- more.
INSERT INTO relationship (kind, project_id, organization_id, role, source, captured_by)
SELECT 'project_company', p.id, p.organization_id, 'customer', 'migration:project-companies', 'system'
  FROM project p
 WHERE p.organization_id IS NOT NULL
   AND NOT EXISTS (
     SELECT 1 FROM relationship r
      WHERE r.kind = 'project_company' AND r.project_id = p.id
        AND r.organization_id = p.organization_id AND r.archived_at IS NULL);

-- The anchor column stops being the answer, so it stops being required. It is
-- NOT dropped here: it is what the backfill above was read from, and a
-- deployment that has to go back needs it to still hold the value. A later
-- migration removes it once nothing reads it.
ALTER TABLE project ALTER COLUMN organization_id DROP NOT NULL;

-- A deal and its project must still agree about the company — but "the
-- project's company" is now any of them. The old trigger compared the deal's
-- company with the project's single anchor, which after this migration is a
-- column nothing maintains.
CREATE OR REPLACE FUNCTION assert_deal_project_same_org() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.project_id IS NULL THEN
    RETURN NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM relationship r
    WHERE r.kind = 'project_company'
      AND r.project_id = NEW.project_id
      AND r.organization_id IS NOT DISTINCT FROM NEW.organization_id
      AND r.archived_at IS NULL
  ) THEN
    RAISE EXCEPTION 'the deal names a company that is not on this project'
      USING ERRCODE = 'check_violation', CONSTRAINT = 'deal_project_same_org';
  END IF;
  RETURN NULL;
END;
$$;
