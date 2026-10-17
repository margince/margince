-- A company may hold several accounting customers.
--
-- A business is often billed under more than one customer record, and a company
-- merge brings two linked companies onto one survivor. The unique index allowed
-- one live link per company and connection, so the merge had to leave the
-- loser's link behind, and the next sync moved its invoices back to the
-- archived company. A customer still belongs to one company at a time
-- (finance_customer_link_external_ux is unchanged).
--
-- The index stays, without uniqueness, for the summary's read by company.
SET LOCAL lock_timeout = '3s';
DROP INDEX finance_customer_link_company_ux;
CREATE INDEX finance_customer_link_company_ix ON finance_customer_link
    USING btree (connection_id, company_id) WHERE (archived_at IS NULL);
