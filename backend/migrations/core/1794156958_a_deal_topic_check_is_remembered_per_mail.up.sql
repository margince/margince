-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET lock_timeout = '3s';

-- The capture deal topic check remembers which candidate deals and prompt it
-- last answered for a mail, so the hourly passes do not ask the model again.
ALTER TABLE activity ADD COLUMN deal_topic_digest text;
ALTER TABLE activity ADD COLUMN deal_topic_checked_at timestamptz;
