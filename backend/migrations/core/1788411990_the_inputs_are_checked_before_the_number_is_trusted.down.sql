-- A DROP takes an exclusive lock, so it waits behind any open reader and holds
-- every writer behind itself while it waits.
SET LOCAL lock_timeout = '5s';

DROP TABLE IF EXISTS assurance_resolution;
DROP TABLE IF EXISTS assurance_exception;
DROP TABLE IF EXISTS assurance_source_coverage;
DROP TABLE IF EXISTS assurance_run;
