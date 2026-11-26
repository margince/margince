-- pgmigrate:no-transaction
-- Every holder of an address, live or archived, found by the address. The
-- shared sender rule asks whether any other contact ever held an address
-- (relstrength.AddressProvesSQL), and uq_contact_email_dedupe covers live rows
-- only. contact_email stores the address folded, so the plain column matches.
-- Built concurrently, so contact writes are never blocked. The drop clears an
-- invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_every_holder;
CREATE INDEX CONCURRENTLY idx_contact_email_every_holder ON contact_email (email, contact_id);
