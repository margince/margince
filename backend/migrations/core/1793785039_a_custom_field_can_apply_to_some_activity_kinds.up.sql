-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An activity field may apply to some activity kinds only, so a visit-report
-- field shows on a meeting and not on a call. NULL means every kind, which is
-- what every field that exists applies to today. Only an activity field has a
-- kind to narrow by. The engine checks each kind against the activity kind
-- list, as it checks options. NOT VALID skips the row scan; every row is NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE custom_field ADD COLUMN activity_kinds text[];
ALTER TABLE custom_field ADD CONSTRAINT custom_field_activity_kinds_check
    CHECK (activity_kinds IS NULL OR (object = 'activity' AND cardinality(activity_kinds) > 0)) NOT VALID;
