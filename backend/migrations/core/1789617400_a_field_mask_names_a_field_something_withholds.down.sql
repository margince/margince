-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- The masks this migration deleted are not restored: they named a pair nothing
-- withholds, so there is nothing to restore them TO, and re-inserting a
-- configuration that does nothing is what the up side removed.
ALTER TABLE field_mask DROP CONSTRAINT IF EXISTS field_mask_maskable_fkey;

DROP TABLE IF EXISTS maskable_field;
