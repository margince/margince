-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field DROP CONSTRAINT custom_field_activity_kinds_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_activity_kinds_check
    CHECK (activity_kinds IS NULL OR (object = 'activity' AND cardinality(activity_kinds) > 0)) NOT VALID;
