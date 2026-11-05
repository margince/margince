-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS meeting_readiness;
ALTER TABLE activity DROP COLUMN IF EXISTS agenda_fingerprint;
