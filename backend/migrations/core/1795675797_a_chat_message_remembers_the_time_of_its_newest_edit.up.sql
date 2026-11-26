-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The provider's own edit time of the text a captured chat message now holds.
-- An edit that is not newer than this changes nothing, so a redelivered or
-- reordered report never overwrites newer text. NULL means the provider has
-- reported no edit.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN source_revision timestamptz;
