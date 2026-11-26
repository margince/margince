-- pgmigrate:no-transaction
-- One bounced delivery has at most one send that repeats it. Two reps pressing
-- the button together, or one pressing it twice, reach this index and the
-- second send is refused. Built while deliveries are written.
SET lock_timeout = '3s';
DROP INDEX CONCURRENTLY IF EXISTS comms_outbound_resent_from_uq;
CREATE UNIQUE INDEX CONCURRENTLY comms_outbound_resent_from_uq ON comms_outbound (resent_from) WHERE resent_from IS NOT NULL;
