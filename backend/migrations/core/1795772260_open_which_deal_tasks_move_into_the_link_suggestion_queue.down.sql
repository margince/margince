-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Margince's own deal questions leave the queue; the tasks the up closed stay
-- done.
SET LOCAL lock_timeout = '5s';

DELETE FROM activity_link_suggestion WHERE unit IS NULL;
