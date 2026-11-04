-- pgmigrate:no-transaction
-- Releasing a contact a confidentiality hold kept to its owner reads every
-- ledger row of its addresses. The ledger had an address index only for rows
-- still open, noise or suppressed, so each release read the whole ledger.
--
-- The noise index read the same column for one status, so the full index
-- answers its queries and it goes.
--
-- Built concurrently, so capture's writes are never blocked for the build. The
-- drop ahead of the build clears an invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS capture_pending_counterparty_email_ix;
CREATE INDEX CONCURRENTLY capture_pending_counterparty_email_ix ON capture_pending_counterparty (email);
DROP INDEX CONCURRENTLY IF EXISTS idx_capture_pending_counterparty_noise;
