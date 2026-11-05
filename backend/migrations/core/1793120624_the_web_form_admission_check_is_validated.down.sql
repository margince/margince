-- Put the check back present and unvalidated, as this migration found it.
-- Postgres has no ALTER that un-validates, so it is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form
    DROP CONSTRAINT web_form_admission,
    ADD CONSTRAINT web_form_admission CHECK (
        (sender = 'browser' AND secret_ref IS NULL AND cardinality(allowed_origins) > 0)
        OR (sender <> 'browser' AND (secret_ref IS NOT NULL OR sender = 'webflow')
            AND cardinality(allowed_origins) = 0)) NOT VALID;
