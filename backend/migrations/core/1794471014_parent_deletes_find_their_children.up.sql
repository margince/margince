-- pgmigrate:no-transaction
-- Parent deletes must find children, including closed and historical rows.
-- Concurrent builds keep writes running. The drops repair failed builds.
DROP INDEX CONCURRENTLY IF EXISTS sales_initiative_enrolment_initiative_idx;
CREATE INDEX CONCURRENTLY sales_initiative_enrolment_initiative_idx ON sales_initiative_enrolment (initiative_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_customer_link_directory_idx;
CREATE INDEX CONCURRENTLY finance_customer_link_directory_idx ON finance_customer_link (connection_id, external_customer_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_credit_owner_idx;
CREATE INDEX CONCURRENTLY finance_invoice_credit_owner_idx ON finance_invoice (credits_invoice_id, connection_id, company_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_invoice_customer_owner_idx;
CREATE INDEX CONCURRENTLY finance_invoice_customer_owner_idx ON finance_invoice (customer_link_id, connection_id, company_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_customer_owner_idx;
CREATE INDEX CONCURRENTLY finance_payment_customer_owner_idx ON finance_payment (customer_link_id, connection_id, company_id);
DROP INDEX CONCURRENTLY IF EXISTS finance_payment_invoice_owner_idx;
CREATE INDEX CONCURRENTLY finance_payment_invoice_owner_idx ON finance_payment (invoice_id, connection_id, company_id);
