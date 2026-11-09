-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE passport DROP COLUMN work_areas;
ALTER TABLE oauth_grant DROP COLUMN work_areas;
ALTER TABLE oauth_authorization_code DROP COLUMN work_areas;
