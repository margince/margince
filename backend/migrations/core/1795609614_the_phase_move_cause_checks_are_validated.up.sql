-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the two checks the migration before this one added NOT VALID.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail:
-- the columns were created there with no default, so each row holds NULL.
SET LOCAL lock_timeout = '5s';

ALTER TABLE project_phase_history VALIDATE CONSTRAINT project_phase_history_cause_kind_check;
ALTER TABLE project_phase_history VALIDATE CONSTRAINT project_phase_history_cause_pairs;
