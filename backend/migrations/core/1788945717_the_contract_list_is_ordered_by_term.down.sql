SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS contract_account_ix;

CREATE INDEX contract_account_ix ON contract
    USING btree (organization_id, created_at DESC, id DESC)
    WHERE (archived_at IS NULL);
