-- Put both constraints back present and unvalidated. Postgres has no ALTER that
-- un-validates, so each is dropped and re-added NOT VALID, without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    DROP CONSTRAINT contact_narrowing_reason_check,
    ADD CONSTRAINT contact_narrowing_reason_check CHECK (narrowing_reason IN (
        'awaiting_verdict', 'outbound_no_answer', 'confidentiality_hold', 'advisor', 'human_decided'))
        NOT VALID,
    DROP CONSTRAINT contact_narrowing_reason_only_when_owner,
    ADD CONSTRAINT contact_narrowing_reason_only_when_owner CHECK (
        narrowing_reason IS NULL OR visibility = 'owner') NOT VALID;
