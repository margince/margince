-- The lawful-basis list names no country. `vn_subject_agreement` was the
-- subject's own agreement under one market's advertising law, which core must
-- not know. Both tables take the same list, the Go set `commsauthz.Basis`; a
-- communication_basis row never carried `consent` before (consent lives in
-- consent_event), so its list gains it.
--
-- No row is rewritten. A communication_decision row is evidence of the basis a
-- decision was made under, and relabelling another basis as consent would
-- falsify it. The new rules are NOT VALID; the next migration validates each
-- one when no row holds the retired value, and drops it with a notice when one
-- does. No code path wrote the value, so the second case is not expected.
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
