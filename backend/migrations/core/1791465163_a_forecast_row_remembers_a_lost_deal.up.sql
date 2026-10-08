-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A lost deal is in no reading, so a movement between two snapshots could only
-- call its departure "pushed out". The row keeps the fact so the waterfall can
-- name the loss.
SET LOCAL lock_timeout = '3s';

ALTER TABLE forecast_contribution ADD COLUMN in_lost boolean DEFAULT false NOT NULL;
