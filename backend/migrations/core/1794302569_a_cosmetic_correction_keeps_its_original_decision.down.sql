-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP TRIGGER communication_instruction_wording_correction ON communication_instruction;
DROP FUNCTION guard_instruction_wording_correction();
ALTER TABLE communication_instruction DROP COLUMN corrected_wording, DROP COLUMN acknowledged_scope;
