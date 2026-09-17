SET LOCAL lock_timeout = '3s';

-- The HubSpot connector is not built and will not be. It was declared in the
-- column's CHECK as the shape a future connector would arrive in; nothing
-- writes it, and a vocabulary that admits a source nobody can produce invites a
-- caller to name one.
--
-- The DELETE is the same shape the mirror's retirement used and for the same
-- reason: ADD CONSTRAINT validates existing rows, so a run recorded under a
-- connector the CHECK no longer admits would refuse the migration rather than
-- the value. No product path writes 'hubspot' into this column, so on every
-- installation this deletes nothing.
DELETE FROM import_record_map WHERE source_system = 'hubspot';
DELETE FROM import_run WHERE connector = 'hubspot';

ALTER TABLE import_run DROP CONSTRAINT IF EXISTS import_run_connector_check;
ALTER TABLE import_run ADD CONSTRAINT import_run_connector_check
    CHECK (connector IN ('csv', 'salesforce'));
