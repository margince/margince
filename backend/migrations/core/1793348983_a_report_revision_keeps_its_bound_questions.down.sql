SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition_revision DROP CONSTRAINT IF EXISTS report_definition_revision_question_bindings_is_an_object;
ALTER TABLE report_definition_revision DROP COLUMN IF EXISTS question_bindings;
