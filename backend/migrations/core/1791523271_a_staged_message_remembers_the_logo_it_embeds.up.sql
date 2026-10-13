-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The logo a staged message's markup embeds, so a retry sends the same picture
-- the first attempt did rather than whichever logo the workspace has since.
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound ADD COLUMN inline_logo_key text;
