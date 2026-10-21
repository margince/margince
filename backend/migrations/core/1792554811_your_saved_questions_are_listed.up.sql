-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A reader lists their own saved questions, newest first, a page at a time.
-- Two runs can share a timestamp, so the page boundary is (created_at, id).
SET LOCAL lock_timeout = '3s';

DROP INDEX idx_report_run_asker;
CREATE INDEX idx_report_run_asker ON report_run (asked_by, created_at DESC, id DESC);
