-- A rep can record that two external people work together — the asserted
-- person↔person edge the observed projection can only suggest.
--
-- works_with is a `relationship` row, not a new table: that table already
-- models every asserted edge with the same role, source, audit and archive
-- semantics, and a second table would be a second answer to "who is connected
-- to whom". The second person lands in a new counterparty_person_id column,
-- the same move counterparty_org_id made for the partner kinds.
--
-- Every shape constraint is restated, not only the new one. The older shapes
-- enumerate which columns their kind must leave NULL, and a column they
-- predate is a column they do not mention — without the restatement an
-- employment row could quietly carry a counterparty person no reader expects.

SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship
  ADD COLUMN counterparty_person_id uuid;

-- The same referential contract every other endpoint column carries: a person
-- hard-deleted by erasure takes their edges with them.
ALTER TABLE relationship
  ADD CONSTRAINT relationship_counterparty_person_id_fkey
  FOREIGN KEY (counterparty_person_id) REFERENCES person(id) ON DELETE CASCADE;

ALTER TABLE relationship
  DROP CONSTRAINT relationship_kind_check;

ALTER TABLE relationship
  ADD CONSTRAINT relationship_kind_check CHECK (kind IN (
    'employment', 'deal_stakeholder', 'partner_of', 'referred_by',
    'co_sell_with', 'project_stakeholder', 'project_company', 'works_with'));

-- A works_with edge names two DIFFERENT people and nothing else. It is
-- undirected in fact; both orders are admitted and the unique index below is
-- what keeps one pair one row.
ALTER TABLE relationship
  ADD CONSTRAINT rel_works_with_shape CHECK (
    (kind <> 'works_with') OR (
      person_id IS NOT NULL AND counterparty_person_id IS NOT NULL
      AND person_id <> counterparty_person_id
      AND organization_id IS NULL AND counterparty_org_id IS NULL
      AND deal_id IS NULL AND project_id IS NULL));

ALTER TABLE relationship
  DROP CONSTRAINT rel_employment_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_employment_shape CHECK (
    (kind <> 'employment') OR (
      person_id IS NOT NULL AND organization_id IS NOT NULL
      AND deal_id IS NULL AND project_id IS NULL AND counterparty_person_id IS NULL));

ALTER TABLE relationship
  DROP CONSTRAINT rel_partner_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_partner_shape CHECK (
    (kind NOT IN ('partner_of', 'referred_by', 'co_sell_with')) OR (
      organization_id IS NOT NULL AND counterparty_org_id IS NOT NULL
      AND organization_id <> counterparty_org_id
      AND person_id IS NULL AND deal_id IS NULL AND project_id IS NULL
      AND counterparty_person_id IS NULL));

ALTER TABLE relationship
  DROP CONSTRAINT rel_project_stakeholder_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_project_stakeholder_shape CHECK (
    (kind <> 'project_stakeholder') OR (
      project_id IS NOT NULL AND person_id IS NOT NULL
      AND organization_id IS NULL AND counterparty_org_id IS NULL
      AND deal_id IS NULL AND counterparty_person_id IS NULL));

ALTER TABLE relationship
  DROP CONSTRAINT rel_stakeholder_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_stakeholder_shape CHECK (
    (kind <> 'deal_stakeholder') OR (
      deal_id IS NOT NULL AND person_id IS NOT NULL
      AND organization_id IS NULL AND project_id IS NULL
      AND counterparty_person_id IS NULL));

ALTER TABLE relationship
  DROP CONSTRAINT rel_project_company_shape;
ALTER TABLE relationship
  ADD CONSTRAINT rel_project_company_shape CHECK (
    (kind <> 'project_company') OR (
      project_id IS NOT NULL AND organization_id IS NOT NULL
      AND person_id IS NULL AND counterparty_org_id IS NULL AND deal_id IS NULL
      AND counterparty_person_id IS NULL));

-- One live edge per pair, whichever order it was recorded in: naming the same
-- two people twice is not two facts.
CREATE UNIQUE INDEX uq_rel_works_with
  ON relationship (LEAST(person_id, counterparty_person_id),
                   GREATEST(person_id, counterparty_person_id))
  WHERE kind = 'works_with' AND archived_at IS NULL;

-- Both ends are read: "who does this person work with" asks either column.
CREATE INDEX idx_rel_works_with_person
  ON relationship (person_id)
  WHERE kind = 'works_with' AND archived_at IS NULL;
CREATE INDEX idx_rel_works_with_counterparty
  ON relationship (counterparty_person_id)
  WHERE kind = 'works_with' AND archived_at IS NULL;
