-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form
    DROP COLUMN privacy_notice_url,
    DROP COLUMN privacy_notice;
