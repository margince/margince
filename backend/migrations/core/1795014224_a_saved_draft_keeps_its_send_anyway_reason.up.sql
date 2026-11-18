-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A saved draft keeps the reason its author typed for sending past a refusal,
-- so a reopened composer still holds it. It is the author's alone, as the
-- draft is, and grants nothing until they send with it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE mail_draft ADD COLUMN IF NOT EXISTS override_explanation text;
