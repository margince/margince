-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A digest of the reader's reply and meeting snoozes that had lifted when the
-- answer was read, NULL when none had. A lift writes nothing, so the cache ends
-- an answer once the digest it would read now differs.
ALTER TABLE home_read_cache ADD COLUMN lifted_snooze_digest bytea;
