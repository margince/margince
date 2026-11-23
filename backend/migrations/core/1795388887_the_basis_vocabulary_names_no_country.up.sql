-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- The lawful-basis list names no country. `vn_subject_agreement` was the
-- subject's own agreement under one market's advertising law, which core must
-- not know. Both tables keep their list minus that value.
--
-- No row is rewritten or refused. A stored basis is evidence of what a decision
-- was made under, and relabelling it would falsify that record. The rules are
-- NOT VALID; the next migration settles each one against the rows it finds.

ALTER TABLE communication_decision DROP CONSTRAINT communication_decision_basis;
ALTER TABLE communication_decision
    ADD CONSTRAINT communication_decision_basis
        CHECK (basis IS NULL OR basis = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'consent'::text, 'existing_customer_exception'::text
        ])) NOT VALID;

ALTER TABLE communication_basis DROP CONSTRAINT communication_basis_kind;
ALTER TABLE communication_basis
    ADD CONSTRAINT communication_basis_kind
        CHECK (kind = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'existing_customer_exception'::text
        ])) NOT VALID;
