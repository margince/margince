-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- avatar_removed_at keeps a login provider from filling a picture the member deleted; only
-- an upload clears it. The checks are validated in the next migration, outside this lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
    ADD COLUMN avatar_object_key text,
    ADD COLUMN avatar_origin text,
    ADD COLUMN avatar_removed_at timestamptz,
    ADD CONSTRAINT app_user_avatar_origin_check
        CHECK (avatar_origin IN ('upload', 'google')) NOT VALID,
    ADD CONSTRAINT app_user_avatar_names_its_origin
        CHECK ((avatar_object_key IS NULL) = (avatar_origin IS NULL)) NOT VALID;
