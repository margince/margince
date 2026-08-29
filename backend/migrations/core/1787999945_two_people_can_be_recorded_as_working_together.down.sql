SET LOCAL lock_timeout = '3s';
DROP INDEX uq_rel_works_with;
DROP INDEX idx_rel_works_with_person;
DROP INDEX idx_rel_works_with_counterparty;
ALTER TABLE relationship DROP CONSTRAINT rel_works_with_shape;
ALTER TABLE relationship DROP CONSTRAINT relationship_kind_check;
ALTER TABLE relationship
  ADD CONSTRAINT relationship_kind_check CHECK (kind IN (
    'employment', 'deal_stakeholder', 'partner_of', 'referred_by',
    'co_sell_with', 'project_stakeholder', 'project_company'));

-- The restated shapes name the column being dropped, so they go back to their
-- prior texts before the column can go.
ALTER TABLE relationship DROP CONSTRAINT rel_employment_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_employment_shape CHECK (
    (kind <> 'employment') OR (
      person_id IS NOT NULL AND organization_id IS NOT NULL
      AND deal_id IS NULL AND project_id IS NULL));
ALTER TABLE relationship DROP CONSTRAINT rel_partner_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_partner_shape CHECK (
    (kind NOT IN ('partner_of', 'referred_by', 'co_sell_with')) OR (
      organization_id IS NOT NULL AND counterparty_org_id IS NOT NULL
      AND organization_id <> counterparty_org_id
      AND person_id IS NULL AND deal_id IS NULL AND project_id IS NULL));
ALTER TABLE relationship DROP CONSTRAINT rel_project_stakeholder_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_project_stakeholder_shape CHECK (
    (kind <> 'project_stakeholder') OR (
      project_id IS NOT NULL AND person_id IS NOT NULL
      AND organization_id IS NULL AND counterparty_org_id IS NULL
      AND deal_id IS NULL));
ALTER TABLE relationship DROP CONSTRAINT rel_stakeholder_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_stakeholder_shape CHECK (
    (kind <> 'deal_stakeholder') OR (
      deal_id IS NOT NULL AND person_id IS NOT NULL
      AND organization_id IS NULL AND project_id IS NULL));
ALTER TABLE relationship DROP CONSTRAINT rel_project_company_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_project_company_shape CHECK (
    (kind <> 'project_company') OR (
      project_id IS NOT NULL AND organization_id IS NOT NULL
      AND person_id IS NULL AND counterparty_org_id IS NULL AND deal_id IS NULL));

ALTER TABLE relationship DROP CONSTRAINT relationship_counterparty_person_id_fkey;
ALTER TABLE relationship DROP COLUMN counterparty_person_id;
