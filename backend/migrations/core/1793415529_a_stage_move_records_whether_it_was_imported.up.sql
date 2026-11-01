-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A stage move says whether it was made in Margince or an import brought it
-- in, so a reading of how this team sells can leave brought-in history out. A row
-- written before this column has no answer and stays NULL: nothing links it to
-- the audit row that would tell.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history
    ADD COLUMN provenance    text,
    ADD COLUMN import_run_id uuid;

-- import_run_id holds no foreign key, as audit_log.batch_id holds none: the
-- import tables are made by the custom migrations, which run after these.
-- Added NOT VALID and validated in the next file, so the scan does not hold
-- every stage move.
ALTER TABLE deal_stage_history
    ADD CONSTRAINT deal_stage_history_provenance_check
        CHECK (provenance IS NULL OR provenance IN ('recorded', 'imported')) NOT VALID,
    ADD CONSTRAINT deal_stage_history_import_run_pairing
        CHECK (CASE provenance WHEN 'imported' THEN import_run_id IS NOT NULL
                                  ELSE import_run_id IS NULL END) NOT VALID;
