-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET lock_timeout = '3s';

ALTER TABLE activity DROP COLUMN deal_topic_checked_at;
ALTER TABLE activity DROP COLUMN deal_topic_digest;
