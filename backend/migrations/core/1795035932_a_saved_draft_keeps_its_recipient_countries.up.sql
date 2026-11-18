-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A saved draft keeps the countries its author stated or cleared for its
-- recipients, so a reopened composer still holds them. NULL is none stated.
SET LOCAL lock_timeout = '3s';

ALTER TABLE mail_draft ADD COLUMN IF NOT EXISTS recipient_countries jsonb;
