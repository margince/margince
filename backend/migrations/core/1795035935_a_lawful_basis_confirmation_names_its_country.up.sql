-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The recipient's country a lawful-basis confirmation was given under, so a
-- send judged under another country cannot spend it. NULL on a row written
-- before this column; such a row is never spent.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lawful_basis_confirmation ADD COLUMN IF NOT EXISTS country text;
