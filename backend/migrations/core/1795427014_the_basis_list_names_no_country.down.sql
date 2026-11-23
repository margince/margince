-- Restores the retired value to both lists. The rules are NOT VALID: rollback
-- restores them for new rows and does not scan the rows already there.
-- Either rule may be a different check by now: the validating migration
-- replaces one that a row broke.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_basis
    DROP CONSTRAINT IF EXISTS communication_basis_kind,
    ADD CONSTRAINT communication_basis_kind
        CHECK (kind = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'existing_customer_exception'::text, 'vn_subject_agreement'::text
        ])) NOT VALID;

ALTER TABLE communication_decision
    DROP CONSTRAINT IF EXISTS communication_decision_basis,
    ADD CONSTRAINT communication_decision_basis
        CHECK (basis IS NULL OR basis = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'consent'::text, 'existing_customer_exception'::text,
            'vn_subject_agreement'::text
        ])) NOT VALID;
