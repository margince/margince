-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Every source_id is NULL until a caller writes one, so each scan below passes.
-- VALIDATE takes only SHARE UPDATE EXCLUSIVE, which lets reads and writes go on.
SET LOCAL lock_timeout = '2s';

ALTER TABLE contact VALIDATE CONSTRAINT contact_source_id_needs_a_source;
ALTER TABLE contact VALIDATE CONSTRAINT contact_source_id_length;
ALTER TABLE company VALIDATE CONSTRAINT company_source_id_needs_a_source;
ALTER TABLE company VALIDATE CONSTRAINT company_source_id_length;
ALTER TABLE deal VALIDATE CONSTRAINT deal_source_id_needs_a_source;
ALTER TABLE deal VALIDATE CONSTRAINT deal_source_id_length;
ALTER TABLE project VALIDATE CONSTRAINT project_source_id_needs_a_source;
ALTER TABLE project VALIDATE CONSTRAINT project_source_id_length;
