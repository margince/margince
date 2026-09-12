SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS rel_billing_contact_by_contact;
DROP INDEX IF EXISTS rel_billing_contact_by_company;
DROP INDEX IF EXISTS uq_rel_billing_contact;

ALTER TABLE relationship DROP CONSTRAINT IF EXISTS rel_billing_contact_role;
ALTER TABLE relationship DROP CONSTRAINT IF EXISTS rel_billing_contact_shape;

DELETE FROM relationship WHERE kind = 'billing_contact';

ALTER TABLE relationship DROP CONSTRAINT relationship_kind_check;

ALTER TABLE relationship ADD CONSTRAINT relationship_kind_check
    CHECK (kind = ANY (ARRAY[
        'employment', 'deal_stakeholder', 'partner_of', 'referred_by',
        'co_sell_with', 'project_stakeholder', 'project_company', 'works_with'
    ]));
