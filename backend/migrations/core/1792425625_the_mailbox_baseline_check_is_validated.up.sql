-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the baseline CHECK the previous migration added NOT VALID, in a
-- transaction of its own so the scan runs under SHARE UPDATE EXCLUSIVE. Every
-- stored row passes: the four columns were all NULL when they were added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection VALIDATE CONSTRAINT capture_connection_history_baseline_check;
