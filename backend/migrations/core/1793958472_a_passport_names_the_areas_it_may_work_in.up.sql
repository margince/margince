-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- NULL admits every area of work, those added later included, so every passport
-- and connection that exists keeps the reach it had. A list admits only its areas.
-- The code, the grant and the passport each carry it, as they carry the scopes.
SET LOCAL lock_timeout = '3s';

ALTER TABLE passport ADD COLUMN work_areas text[];
ALTER TABLE oauth_grant ADD COLUMN work_areas text[];
ALTER TABLE oauth_authorization_code ADD COLUMN work_areas text[];
