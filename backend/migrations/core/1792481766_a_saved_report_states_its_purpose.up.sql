-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What a saved report is for, in the author's words. It sits beside the name and
-- follows the name's rules: not versioned, and changed only while no draft is
-- open. Its length is checked where the name's is, in the reporting service.
SET LOCAL lock_timeout = '3s';

ALTER TABLE report_definition ADD COLUMN description text;
