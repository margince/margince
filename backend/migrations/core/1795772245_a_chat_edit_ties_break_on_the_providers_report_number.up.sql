-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Two edits of one chat message can carry the same provider edit time, because
-- a provider stamps whole seconds. The provider's own sequence number for the
-- report breaks the tie, so the later report wins whichever arrives first.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity ADD COLUMN source_revision_seq bigint NOT NULL DEFAULT 0;
