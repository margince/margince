SET LOCAL lock_timeout = '3s';
-- A name-sorted list orders by the first 256 characters of the name, so the
-- cursor stays small however long a name is (storekit sortKeyPrefixChars). The
-- keyset indexes carry the same expression, or those pages would sort the table.
-- Plain builds: the runner holds each migration in one transaction, where
-- CONCURRENTLY cannot run.
DROP INDEX idx_company_name_keyset;
DROP INDEX idx_company_name_keyset_desc;
DROP INDEX idx_contact_name_keyset;
DROP INDEX idx_contact_name_keyset_desc;
CREATE INDEX idx_company_name_keyset ON company (left(display_name, 256), created_at DESC, id DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_company_name_keyset_desc ON company (left(display_name, 256) DESC NULLS LAST, created_at DESC, id DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_contact_name_keyset ON contact (left(full_name, 256), created_at DESC, id DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_contact_name_keyset_desc ON contact (left(full_name, 256) DESC NULLS LAST, created_at DESC, id DESC) WHERE archived_at IS NULL;
