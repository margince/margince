-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE email_signature DROP COLUMN IF EXISTS phone;
ALTER TABLE email_signature DROP COLUMN IF EXISTS title;
