-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_invoice_owner_ix;
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_customer_link_ix;
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_credits_owner_ix;
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_customer_link_ix;
DROP INDEX CONCURRENTLY IF EXISTS finance_customer_link_directory_ix;
