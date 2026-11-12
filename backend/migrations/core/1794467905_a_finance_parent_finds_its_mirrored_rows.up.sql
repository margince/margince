-- pgmigrate:no-transaction
-- Each composite key the finance mirror validates gets an index in its own
-- column order. A company merge rewrites the company on a link and its
-- invoices, and the installation reset deletes them row by row. Both make
-- Postgres look up the referencing rows, which without these is a scan per row.
-- Built concurrently, so a sync is never blocked. Each drop clears an invalid
-- index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS finance_customer_link_directory_ix;
CREATE INDEX CONCURRENTLY finance_customer_link_directory_ix
    ON finance_customer_link (connection_id, external_customer_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_customer_link_ix;
CREATE INDEX CONCURRENTLY finance_invoice_customer_link_ix
    ON finance_invoice (customer_link_id, connection_id, company_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_credits_owner_ix;
CREATE INDEX CONCURRENTLY finance_invoice_credits_owner_ix
    ON finance_invoice (credits_invoice_id, connection_id, company_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_customer_link_ix;
CREATE INDEX CONCURRENTLY finance_payment_customer_link_ix
    ON finance_payment (customer_link_id, connection_id, company_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_invoice_owner_ix;
CREATE INDEX CONCURRENTLY finance_payment_invoice_owner_ix
    ON finance_payment (invoice_id, connection_id, company_id);
