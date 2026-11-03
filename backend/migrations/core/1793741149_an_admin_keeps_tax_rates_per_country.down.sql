-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No tax rate sheet and no product categories. The system roles lose the
-- grant the up gave them, so a later up grants it again rather than finding a
-- stale key and leaving it alone.
SET LOCAL lock_timeout = '3s';

UPDATE role SET permissions = permissions #- '{objects,tax_rate}'
    WHERE is_system AND permissions ? 'objects' AND (permissions -> 'objects') ? 'tax_rate';

ALTER TABLE product DROP COLUMN IF EXISTS tax_category;
DROP TABLE IF EXISTS tax_rate;
DROP FUNCTION IF EXISTS tax_rate_refuse_change();
