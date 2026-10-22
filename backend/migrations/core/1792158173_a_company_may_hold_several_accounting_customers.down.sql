-- Restoring uniqueness fails while any company holds two live links on one
-- connection. That is the honest reverse: those links have to be undone by
-- hand first, because which of them to keep is a decision about money.
SET LOCAL lock_timeout = '3s';
DROP INDEX finance_customer_link_company_ix;
CREATE UNIQUE INDEX finance_customer_link_company_ux ON finance_customer_link
    USING btree (connection_id, company_id) WHERE (archived_at IS NULL);
