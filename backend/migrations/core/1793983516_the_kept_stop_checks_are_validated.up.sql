-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validate the two checks the migration before this one added NOT VALID. Its
-- own file, so each scan runs under SHARE UPDATE EXCLUSIVE. Neither can fail:
-- every row was backfilled to a listed value, and the target check is looser
-- than the one it replaced.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression VALIDATE CONSTRAINT communication_suppression_recorded_through;
ALTER TABLE communication_suppression VALIDATE CONSTRAINT communication_suppression_names_a_target;
