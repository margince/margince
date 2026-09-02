SET LOCAL lock_timeout = '3s';

-- verdict_reason is untouched by the up migration and still carries the
-- deciding rule, so dropping the list loses only the secondary reasons. What
-- that costs is the widening path, which the down migration's tree does not
-- have.
ALTER TABLE capture_import DROP COLUMN verdict_reasons;
