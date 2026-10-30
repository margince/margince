-- The questions a report revision's cards are bound to, keyed by the opaque
-- reference each card carries. A bound question holds its filters, which
-- name records a reader may not open, so it lives here and never in the
-- selection every reader of the report receives.
SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition_revision
  ADD COLUMN question_bindings jsonb NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE report_definition_revision
  ADD CONSTRAINT report_definition_revision_question_bindings_is_an_object
  CHECK (jsonb_typeof(question_bindings) = 'object');
