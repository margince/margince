-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_body_completeness_check;
ALTER TABLE activity DROP COLUMN IF EXISTS body_completeness;
