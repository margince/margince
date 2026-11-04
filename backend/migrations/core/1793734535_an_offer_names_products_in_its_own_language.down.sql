-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Products have one name again, and offers no language.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer DROP COLUMN IF EXISTS language;
DROP TABLE IF EXISTS product_translation;
