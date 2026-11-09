-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
    DROP CONSTRAINT app_user_avatar_origin_check,
    ADD CONSTRAINT app_user_avatar_origin_check
        CHECK (avatar_origin IN ('upload', 'google')) NOT VALID,
    DROP CONSTRAINT app_user_avatar_names_its_origin,
    ADD CONSTRAINT app_user_avatar_names_its_origin
        CHECK ((avatar_object_key IS NULL) = (avatar_origin IS NULL)) NOT VALID;
