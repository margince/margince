-- Forward-only connector vocabulary: re-admitting a value cannot restore runs
-- recorded under it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run DROP CONSTRAINT IF EXISTS import_run_connector_check;
ALTER TABLE import_run ADD CONSTRAINT import_run_connector_check
    CHECK (connector IN ('csv', 'hubspot', 'salesforce'));
