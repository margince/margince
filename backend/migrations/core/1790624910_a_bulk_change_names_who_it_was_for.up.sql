-- requested_by names a passport for an agent, and an OAuth refresh mints a new
-- one, so it cannot say whose batch this is after a refresh. requested_for is
-- the human the change was made for, on the website or through an agent, and
-- is what "the requester's own batch" reads. A connection's passports all act
-- for one human, so it covers the refreshed agent too.
SET LOCAL lock_timeout = '5s';

ALTER TABLE bulk_operation ADD COLUMN requested_for uuid;
