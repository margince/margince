SET LOCAL lock_timeout = '3s';

CREATE INDEX IF NOT EXISTS rel_billing_contact_by_company
    ON relationship (company_id, role)
    WHERE kind = 'billing_contact' AND archived_at IS NULL;

CREATE INDEX IF NOT EXISTS rel_billing_contact_by_contact
    ON relationship (contact_id)
    WHERE kind = 'billing_contact' AND archived_at IS NULL;
