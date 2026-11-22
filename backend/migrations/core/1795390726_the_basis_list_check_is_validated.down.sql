-- Put both checks back unvalidated, the state the migration before this one
-- left them in. Postgres cannot un-validate, so each is dropped and added
-- again NOT VALID, which scans nothing.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_basis
    DROP CONSTRAINT communication_basis_kind,
    ADD CONSTRAINT communication_basis_kind
        CHECK (kind = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'consent'::text, 'existing_customer_exception'::text
        ])) NOT VALID;

ALTER TABLE communication_decision
    DROP CONSTRAINT communication_decision_basis,
    ADD CONSTRAINT communication_decision_basis
        CHECK (basis IS NULL OR basis = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'consent'::text, 'existing_customer_exception'::text
        ])) NOT VALID;
