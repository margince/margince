SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_company_import_legal_name;
DROP INDEX IF EXISTS idx_company_import_display_name;
DROP FUNCTION IF EXISTS f_fold_import_name(text);
