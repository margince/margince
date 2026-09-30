-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- Workspace-scoped category rules cannot survive the narrower constraint.
DELETE FROM capture_exclusion WHERE kind = 'container' AND scope <> 'user';

ALTER TABLE capture_exclusion DROP CONSTRAINT capture_exclusion_container_is_personal;

ALTER TABLE capture_exclusion ADD CONSTRAINT capture_exclusion_container_is_personal
    CHECK (kind <> 'container' OR scope = 'user');
