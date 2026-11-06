-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user VALIDATE CONSTRAINT app_user_avatar_origin_check;
ALTER TABLE app_user VALIDATE CONSTRAINT app_user_avatar_names_its_origin;
