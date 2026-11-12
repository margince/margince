-- pgmigrate:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_invoice_owner_idx;
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_customer_owner_idx;
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_customer_owner_idx;
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_credit_owner_idx;
DROP INDEX CONCURRENTLY IF EXISTS finance_customer_link_directory_idx;
DROP INDEX CONCURRENTLY IF EXISTS sales_initiative_enrolment_initiative_idx;
