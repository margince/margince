-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
    DROP COLUMN avatar_removed_at,
    DROP COLUMN avatar_origin,
    DROP COLUMN avatar_object_key;
