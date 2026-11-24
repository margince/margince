-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- company.quarantined_at has no writer and no reader. Nothing stamps a company as
-- quarantined and nothing offers a review of one. contact.quarantined_at is not
-- touched: capture still writes it, and what to do with it is open.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company DROP COLUMN IF EXISTS quarantined_at;
