-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the check the previous migration added NOT VALID. It is a file of
-- its own so the scan takes only SHARE UPDATE EXCLUSIVE and writers go through.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_sender_override VALIDATE CONSTRAINT capture_sender_override_decision_check;
