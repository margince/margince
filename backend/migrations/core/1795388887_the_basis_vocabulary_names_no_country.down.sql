-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Widens both lists again. Rows the up direction mapped to consent
-- are not restored; this direction exists for a rollback that has not yet run
-- in production. Both are added NOT VALID: a widened list
-- cannot reject a stored row, so nothing validates them.
SET LOCAL lock_timeout = '5s';

ALTER TABLE communication_decision DROP CONSTRAINT communication_decision_basis;
ALTER TABLE communication_decision
    ADD CONSTRAINT communication_decision_basis
        CHECK (basis IS NULL OR basis = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'consent'::text, 'existing_customer_exception'::text,
            'vn_subject_agreement'::text
        ])) NOT VALID;

ALTER TABLE communication_basis DROP CONSTRAINT communication_basis_kind;
ALTER TABLE communication_basis
    ADD CONSTRAINT communication_basis_kind
        CHECK (kind = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'existing_customer_exception'::text, 'vn_subject_agreement'::text
        ])) NOT VALID;
