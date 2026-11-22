-- The lawful-basis list names no country. `vn_subject_agreement` was the
-- subject's own agreement under one market's advertising law, which core must
-- not know; the grant that rests on it is `consent`, and the pack that grants
-- it supplies the citation.
--
-- Both tables take the same list, the Go set `commsauthz.Basis`. A
-- communication_basis row never carried `consent` before (consent lives in
-- consent_event), so the list there gains it; a row that held the retired
-- value moves onto it. No code path wrote the value, so the UPDATEs touch
-- no rows on a database that only ran this code.
--
-- The new rules are NOT VALID and are validated by the next migration, which
-- runs under a lighter lock because it is a transaction of its own.
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
UPDATE communication_basis SET kind = 'consent' WHERE kind = 'vn_subject_agreement';

ALTER TABLE communication_decision
    DROP CONSTRAINT communication_decision_basis,
    ADD CONSTRAINT communication_decision_basis
        CHECK (basis IS NULL OR basis = ANY (ARRAY[
            'contract'::text, 'precontract_request'::text,
            'subject_initiated_correspondence'::text, 'legitimate_interests'::text,
            'legal_obligation'::text, 'vital_or_security_interest'::text,
            'consent'::text, 'existing_customer_exception'::text
        ])) NOT VALID;
UPDATE communication_decision SET basis = 'consent' WHERE basis = 'vn_subject_agreement';
