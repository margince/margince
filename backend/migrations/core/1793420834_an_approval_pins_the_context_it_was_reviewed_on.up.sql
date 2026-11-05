-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A digest of the facts outside the target row that an approval was staged on,
-- such as the company a promotion resolves. Nullable with no default, so no row
-- is rewritten; a kind that needs one refuses a row that has none.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval ADD COLUMN context_digest text;
