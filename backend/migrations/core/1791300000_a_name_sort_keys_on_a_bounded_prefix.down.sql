SET LOCAL lock_timeout = '3s';
DROP INDEX idx_company_name_keyset;
DROP INDEX idx_company_name_keyset_desc;
DROP INDEX idx_contact_name_keyset;
DROP INDEX idx_contact_name_keyset_desc;
CREATE INDEX idx_company_name_keyset ON company (display_name, created_at DESC, id DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_company_name_keyset_desc ON company (display_name DESC NULLS LAST, created_at DESC, id DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_contact_name_keyset ON contact (full_name, created_at DESC, id DESC) WHERE archived_at IS NULL;
CREATE INDEX idx_contact_name_keyset_desc ON contact (full_name DESC NULLS LAST, created_at DESC, id DESC) WHERE archived_at IS NULL;
