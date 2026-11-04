-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What became of the mail that carries a set-password link. queued is set when
-- the link is minted to be mailed; sent or failed is the relay's answer. The
-- member roster reads them to show an invitation that never left and to offer
-- a resend. A link an admin copies by hand is never mailed and keeps all three
-- NULL.
SET LOCAL lock_timeout = '3s';

ALTER TABLE auth_token
    ADD COLUMN mail_queued_at timestamptz,
    ADD COLUMN mail_sent_at timestamptz,
    ADD COLUMN mail_failed_at timestamptz;
