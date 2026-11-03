-- A Webflow form may exist before its secret: Webflow issues the secret when
-- its webhook is made, and the webhook needs the form's address first. Until
-- the secret is pasted, the intake refuses every post to it as unsigned.
--
-- NOT VALID, and the next migration validates it under the lighter lock. The
-- rule is looser than the one it replaces, so every row already passes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form
    DROP CONSTRAINT web_form_admission,
    ADD CONSTRAINT web_form_admission CHECK (
        (sender = 'browser' AND secret_ref IS NULL AND cardinality(allowed_origins) > 0)
        OR (sender <> 'browser' AND (secret_ref IS NOT NULL OR sender = 'webflow')
            AND cardinality(allowed_origins) = 0)) NOT VALID;
