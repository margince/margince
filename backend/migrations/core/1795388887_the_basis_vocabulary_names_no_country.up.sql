-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- vn_subject_agreement was the subject's own agreement under a named country's
-- law. Nothing writes it; a stored row is mapped to consent, which is what it is.
-- A decision row is mapped to consent. A communication_basis row has no
-- equivalent kind and is evidence the subject-access export reads, so it is
-- never deleted here: the migration stops and a maintainer decides.
UPDATE communication_decision SET basis = 'consent' WHERE basis = 'vn_subject_agreement';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM communication_basis WHERE kind = 'vn_subject_agreement') THEN
        RAISE EXCEPTION 'communication_basis holds rows of the retired kind vn_subject_agreement; a maintainer must decide how to keep that evidence before this migration can run';
    END IF;
END
$$;

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
