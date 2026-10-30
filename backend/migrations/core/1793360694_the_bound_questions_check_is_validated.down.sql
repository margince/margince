-- Put the constraint back the way this migration found it: present, unvalidated.
-- Postgres has no ALTER that un-validates, so it is dropped and re-added NOT VALID.
SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition_revision
    DROP CONSTRAINT report_definition_revision_question_bindings_is_an_object,
    ADD CONSTRAINT report_definition_revision_question_bindings_is_an_object
        CHECK (jsonb_typeof(question_bindings) = 'object') NOT VALID;
