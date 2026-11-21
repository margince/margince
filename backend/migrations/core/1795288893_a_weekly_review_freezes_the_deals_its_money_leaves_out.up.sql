-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A weekly review's money adds the deals that convert. These columns freeze
-- how many deals it left out, by cause, and the currencies with no rate, so an
-- old review reads the same later. Null means the review was written before
-- the figure said what it leaves out: coverage not assessed.
-- One document per review: {"no_value": n, "no_rate": n, "currencies": ["JPY"]}.
ALTER TABLE weekly_review ADD COLUMN IF NOT EXISTS pipeline_gaps jsonb;
ALTER TABLE team_weekly_review ADD COLUMN IF NOT EXISTS pipeline_gaps jsonb;
