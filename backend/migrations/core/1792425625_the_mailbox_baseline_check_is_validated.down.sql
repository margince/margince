-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_history_baseline_check;
ALTER TABLE capture_connection ADD CONSTRAINT capture_connection_history_baseline_check CHECK (
    (history_baseline_date IS NULL) = (history_baseline_tz IS NULL)
    AND (history_baseline_date IS NULL) = (history_baseline_set_by IS NULL)
    AND (history_baseline_date IS NULL) = (history_baseline_set_at IS NULL)) NOT VALID;
