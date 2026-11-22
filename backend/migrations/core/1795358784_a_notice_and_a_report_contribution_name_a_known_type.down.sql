-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE notice DROP CONSTRAINT IF EXISTS notice_target_type_check;
ALTER TABLE report_edition_contribution DROP CONSTRAINT IF EXISTS report_edition_contribution_source_type_check;
