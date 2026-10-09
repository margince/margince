-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- When capture first saw an employment edge: the earliest message one of the
-- contact's addresses on the employer's domain took part in. Kept apart from
-- started_at, which is the date a person supplied, because a message shows that
-- somebody worked there by its date, not when they started. NULL means capture
-- has no dated evidence, and the account reach walk leaves the edge unbounded.
SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship ADD COLUMN first_observed_at timestamptz;
