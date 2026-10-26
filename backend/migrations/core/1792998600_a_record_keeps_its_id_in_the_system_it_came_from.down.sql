-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Back to a record that names its source system but not its id there.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS uq_contact_source;
DROP INDEX IF EXISTS uq_company_source;
DROP INDEX IF EXISTS uq_deal_source;
DROP INDEX IF EXISTS uq_project_source;

ALTER TABLE contact DROP COLUMN IF EXISTS source_id;
ALTER TABLE company DROP COLUMN IF EXISTS source_id;
ALTER TABLE deal DROP COLUMN IF EXISTS source_id;
ALTER TABLE project DROP COLUMN IF EXISTS source_id;
