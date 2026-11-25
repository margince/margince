-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the version bound the previous migration added NOT VALID. It is a
-- file of its own so the scan takes only SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE extension_ingest_position VALIDATE CONSTRAINT extension_ingest_position_version_bound;
