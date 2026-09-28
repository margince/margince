-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An imported record now carries its author from the create that lands it, so
-- the after-the-fact repair and the ledger that made its re-runs safe are gone.
-- The author columns on the six record tables stay.
SET LOCAL lock_timeout = '3s';

DROP TABLE source_attribution_repair;
