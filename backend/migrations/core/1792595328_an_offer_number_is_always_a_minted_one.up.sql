-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An offer number is always one the mint issued. The mint reads the highest
-- A-<n> to choose the next, so a row in any other shape would be invisible to
-- it, and the next offer would reuse a number a customer already holds.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer ADD CONSTRAINT offer_number_shape CHECK (offer_number ~ '^A-[0-9]+$') NOT VALID;
