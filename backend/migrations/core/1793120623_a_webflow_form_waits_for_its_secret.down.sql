-- NOT VALID, so a Webflow form still waiting for its secret is kept: it takes
-- no post either way.
SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form
    DROP CONSTRAINT web_form_admission,
    ADD CONSTRAINT web_form_admission CHECK (
        (sender = 'browser' AND secret_ref IS NULL AND cardinality(allowed_origins) > 0)
        OR (sender <> 'browser' AND secret_ref IS NOT NULL AND cardinality(allowed_origins) = 0)) NOT VALID;
