-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Rows that STORE the word move back with it, mirroring the up migration.
UPDATE saved_view
   SET query = replace(query::text, '"status"', '"lifecycle"')::jsonb
 WHERE resource = 'companies'
   AND query::text LIKE '%"status"%';

UPDATE field_provenance
   SET field_name = 'lifecycle'
 WHERE field_name = 'status' AND object_type = 'company';

ALTER TABLE company RENAME COLUMN status TO lifecycle;
ALTER TABLE company RENAME CONSTRAINT company_status_check TO company_lifecycle_check;
ALTER INDEX idx_company_status RENAME TO idx_company_lifecycle;
