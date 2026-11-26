-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Source quality counts a lead under the origin it arrived with, as owner
-- credit counts it under the owner it arrived with. An edit to the origin later
-- must not move a lead to another source in a past period.
SET LOCAL lock_timeout = '3s';
ALTER TABLE lead ADD COLUMN IF NOT EXISTS arrival_origin text;

COMMENT ON COLUMN lead.arrival_origin IS
  'The origin when the lead arrived; empty when it arrived with none; null before this was recorded.';
