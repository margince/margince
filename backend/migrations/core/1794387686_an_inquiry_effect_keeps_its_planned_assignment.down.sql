-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM automation_effect_claim WHERE planned IS NOT NULL) THEN
        RAISE EXCEPTION 'Cannot discard durable inquiry assignments';
    END IF;
END $$;
ALTER TABLE automation_effect_claim DROP COLUMN planned;
