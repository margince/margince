-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The check added beside this file is validated here, under SHARE UPDATE
-- EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE meeting_invitation VALIDATE CONSTRAINT meeting_invitation_attention_reason_check;
